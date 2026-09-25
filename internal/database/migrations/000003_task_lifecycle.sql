UPDATE work_nodes SET lifecycle_status = 'closed' WHERE lifecycle_status = 'done';

ALTER TABLE work_nodes
    ADD CONSTRAINT work_nodes_lifecycle_status_check
    CHECK (lifecycle_status IN ('planned', 'active', 'blocked', 'review', 'closed'));

CREATE UNIQUE INDEX work_node_single_performer_idx
    ON work_node_participants(work_node_id)
    WHERE participant_role = 'performer';
