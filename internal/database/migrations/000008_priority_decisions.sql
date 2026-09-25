CREATE TABLE priority_decisions (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    left_root_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    right_root_id uuid NOT NULL REFERENCES work_nodes(id) ON DELETE CASCADE,
    chosen_root_id uuid REFERENCES work_nodes(id) ON DELETE CASCADE,
    decision_basis text NOT NULL CHECK (decision_basis IN ('goal', 'requester', 'unknown')),
    decided_by uuid NOT NULL REFERENCES accounts(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (left_root_id <> right_root_id),
    CHECK (chosen_root_id IS NULL OR chosen_root_id IN (left_root_id, right_root_id))
);

CREATE INDEX priority_decisions_workspace_pair_idx
    ON priority_decisions(workspace_id, left_root_id, right_root_id, created_at DESC);
