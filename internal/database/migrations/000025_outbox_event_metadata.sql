ALTER TABLE outbox_events
    ADD COLUMN workspace_revision bigint,
    ADD COLUMN actor_id uuid REFERENCES actors(id) ON DELETE SET NULL,
    ADD COLUMN correlation_id uuid;

WITH matches AS (
    SELECT o.id AS outbox_id,e.workspace_revision,e.actor_id,e.correlation_id,
           row_number() OVER (PARTITION BY o.id ORDER BY e.occurred_at DESC,e.id DESC) AS rank
    FROM outbox_events o
    JOIN change_events e
      ON e.workspace_id=o.workspace_id
     AND e.event_type=o.event_type
     AND (e.entity_id=o.aggregate_id OR e.root_id=o.aggregate_id)
     AND e.occurred_at<=o.occurred_at
)
UPDATE outbox_events o
SET workspace_revision=m.workspace_revision,
    actor_id=m.actor_id,
    correlation_id=m.correlation_id
FROM matches m
WHERE o.id=m.outbox_id AND m.rank=1;

CREATE INDEX outbox_events_workspace_cursor_idx
    ON outbox_events(workspace_id, occurred_at, id);
