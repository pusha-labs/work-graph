package gateway

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var errMissingPlacementParent = errors.New("placement parent is missing")

func (h *Handler) runRetryWorker(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		for {
			err := h.processDueRetry(ctx)
			if err == pgx.ErrNoRows || ctx.Err() != nil {
				break
			}
			if err != nil {
				h.logger.Error("process gateway retry", "error", err)
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (h *Handler) processDueRetry(ctx context.Context) error {
	tx, err := h.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM gateway_candidates WHERE status='retrying' AND next_attempt_at<=now() ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT 1`)
	item, err := scanCandidate(row)
	if err != nil {
		return err
	}
	lease := time.Now().UTC().Add(45 * time.Second)
	if _, err = tx.Exec(ctx, `UPDATE gateway_candidates SET next_attempt_at=$2,updated_at=now() WHERE id=$1`, item.ID, lease); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	if item.PlacementParentID == nil {
		_, err = h.recordPlacementFailure(ctx, item.ID, errMissingPlacementParent)
		return err
	}
	if _, err = h.commitPlacement(ctx, item, *item.PlacementParentID); err != nil {
		_, recordErr := h.recordPlacementFailure(ctx, item.ID, err)
		return recordErr
	}
	return nil
}
