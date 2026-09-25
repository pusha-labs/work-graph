ALTER TABLE gateway_candidates DROP CONSTRAINT gateway_candidates_status_check;
UPDATE gateway_candidates SET status = 'dead_letter' WHERE status = 'failed';
ALTER TABLE gateway_candidates
    ADD CONSTRAINT gateway_candidates_status_check CHECK (status IN ('unplaced','suggested','placed','retrying','dead_letter')),
    ADD COLUMN placement_parent_id uuid,
    ADD COLUMN attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    ADD COLUMN max_attempts integer NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
    ADD COLUMN next_attempt_at timestamptz,
    ADD COLUMN last_attempt_at timestamptz,
    ADD COLUMN dead_lettered_at timestamptz;

CREATE INDEX gateway_candidates_retry_idx ON gateway_candidates(next_attempt_at)
    WHERE status = 'retrying';
