package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type syncLease struct {
	InstallationID string
	Interval       time.Duration
	Failures       int
}

func (h *Handler) runSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				lease, err := h.claimSync(ctx)
				if errors.Is(err, pgx.ErrNoRows) {
					break
				}
				if err != nil {
					h.logger.Error("claim provider synchronization", "error", err)
					break
				}
				runID := h.startSyncRun(ctx, lease.InstallationID, "scheduled")
				result, syncErr := h.syncJira(ctx, lease.InstallationID)
				h.finishSyncRun(ctx, runID, result, syncErr)
				if err = syncErr; err != nil {
					h.logger.Error("background Jira synchronization", "installation_id", lease.InstallationID, "error", err)
					h.finishScheduledSync(ctx, lease, err)
					continue
				}
				h.finishScheduledSync(ctx, lease, nil)
			}
		}
	}
}

func (h *Handler) claimSync(ctx context.Context) (syncLease, error) {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return syncLease{}, err
	}
	defer tx.Rollback(ctx)
	var lease syncLease
	var intervalSeconds int
	err = tx.QueryRow(ctx, `SELECT s.installation_id::text,s.interval_seconds,s.consecutive_failures
		FROM gateway_sync_schedules s JOIN gateway_installations i ON i.id=s.installation_id
		WHERE s.enabled AND i.installation_status='connected' AND s.next_run_at<=now() AND (s.lease_until IS NULL OR s.lease_until<=now())
		ORDER BY s.next_run_at FOR UPDATE OF s SKIP LOCKED LIMIT 1`).Scan(&lease.InstallationID, &intervalSeconds, &lease.Failures)
	if err != nil {
		return syncLease{}, err
	}
	lease.Interval = time.Duration(intervalSeconds) * time.Second
	if _, err = tx.Exec(ctx, `UPDATE gateway_sync_schedules SET lease_until=now()+interval '2 minutes',last_started_at=now(),updated_at=now() WHERE installation_id=$1`, lease.InstallationID); err != nil {
		return syncLease{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return syncLease{}, err
	}
	return lease, nil
}

func (h *Handler) finishScheduledSync(ctx context.Context, lease syncLease, syncErr error) {
	if syncErr == nil {
		_, err := h.db.Exec(ctx, `UPDATE gateway_sync_schedules SET next_run_at=CASE WHEN continuation_token<>'' THEN now() ELSE now()+($2::bigint*interval '1 second') END,lease_until=NULL,last_completed_at=now(),last_error='',consecutive_failures=0,updated_at=now() WHERE installation_id=$1`, lease.InstallationID, int64(lease.Interval/time.Second))
		if err != nil {
			h.logger.Error("complete provider synchronization", "installation_id", lease.InstallationID, "error", err)
		}
		return
	}
	delay := syncRetryDelay(lease.Interval, lease.Failures)
	message := "Jira synchronization failed; the gateway will retry automatically"
	_, err := h.db.Exec(ctx, `UPDATE gateway_sync_schedules SET next_run_at=now()+($2::bigint*interval '1 second'),lease_until=NULL,last_completed_at=now(),last_error=$3,consecutive_failures=consecutive_failures+1,updated_at=now() WHERE installation_id=$1`, lease.InstallationID, int64(delay/time.Second), message)
	if err == nil {
		_, err = h.db.Exec(ctx, `UPDATE gateway_installations SET last_error=$2,updated_at=now() WHERE id=$1`, lease.InstallationID, message)
	}
	if err != nil {
		h.logger.Error("record provider synchronization failure", "installation_id", lease.InstallationID, "error", fmt.Errorf("%w (sync error: %v)", err, syncErr))
	}
}

func syncRetryDelay(interval time.Duration, failures int) time.Duration {
	delay := interval
	for attempt := 0; attempt <= failures && delay < time.Hour; attempt++ {
		delay *= 2
	}
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
