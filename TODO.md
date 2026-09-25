# Technical TODO

This file records deliberate implementation debt that should not be confused with the product roadmap.

## Rename workflow steps to stages

The product language is now:

- a **Task** defines the desired result;
- a **Task Circle** is the closed responsibility route;
- a **Stage** is one ordered unit of human or automated work inside that circle;
- a **Performer** executes a human stage;
- an **Automation** executes an API, script, or module stage;
- the **Requester** starts the circle and accepts or returns the final result.

The UI uses **stage**, but the current implementation still uses `step` internally. A future compatibility-aware migration should rename:

- the `workflow_steps` table and its columns, constraints, and indexes;
- foreign keys such as `workflow_step_id`;
- API paths such as `/workflow-steps` and JSON fields such as `stepId`, `stepType`, and `stepStatus`;
- Go and TypeScript types, variables, handlers, diagnostics, events, and CSS class names;
- public documentation that describes implementation details.

Do not perform this as a blind search-and-replace. Preserve old API contracts through a versioned transition or compatibility layer, migrate stored event payloads only with an explicit history policy, and update integrations and the Jira gateway together.

Completion criteria:

- new public APIs and schemas consistently use `stage`;
- existing clients receive a documented migration path;
- database migrations are reversible and tested against existing production data;
- historical events remain readable;
- no user-facing `step` terminology remains unless it refers to an ordinary procedural instruction rather than a Task Circle stage.
