# API access for integration services

Work Graph integrations authenticate as workspace-scoped service accounts. A service account is an automation actor of its own: it is not a shared human login, and native changes made with its token are attributed to that actor in tree history.

Workspace owners and administrators manage these identities in **Administration → API access**. The raw token is displayed only when it is created or replaced. Work Graph stores a SHA-256 digest, so a lost token cannot be recovered; replace it instead.

Send the token in the standard authorization header:

```http
Authorization: Bearer wgsa_…
```

The first provider-neutral surface supports:

- `work:read`: discover the account's single workspace; read its current nodes, removed branches, tree history, and historical revisions;
- `work:write`: create, update, move, remove, and restore native branches.

## Idempotent creation

Service accounts must include an `Idempotency-Key` header when creating a native node. Use a stable operation identifier, for example `jira:ABC-123:create`, and keep it unchanged when retrying after a timeout:

```http
POST /api/v1/workspaces/{workspaceId}/nodes
Authorization: Bearer wgsa_…
Idempotency-Key: jira:ABC-123:create
If-Match: "42"
Content-Type: application/json
```

The first accepted request creates the node and stores its result atomically. An identical retry returns the original node with `Idempotency-Replayed: true`; it does not advance the workspace revision or create another history event. Reusing the key with different request content returns `409 Conflict`. Keys are isolated per service account and may contain 1–200 characters.

## Revision preconditions

Every service-account mutation must include the workspace revision it was based on in `If-Match`. The current revision is returned by `GET /api/v1/workspaces` and successful node responses contain their resulting revision. Work Graph advances the revision and applies the mutation atomically only when it still matches.

If another person or integration changed the workspace first, Work Graph returns `412 Precondition Failed`, includes the latest revision in `currentRevision`, and makes no change. The gateway should read the affected state again, reconsider its command, and submit a new operation. It must not blindly replace the header and retry a structurally consequential change.

Missing or invalid `If-Match` on a service mutation returns `428 Precondition Required`. Human browser operations keep their existing behavior and do not require this header.

## Placement recommendations

Before creating a node, a gateway can send its provider-neutral candidate to:

```http
POST /api/v1/workspaces/{workspaceId}/placement-recommendations
Authorization: Bearer wgsa_…
Content-Type: application/json

{
  "title": "Prepare construction site utilities",
  "desiredOutcome": "The ground and utilities are ready",
  "capabilityIds": [],
  "knowledgeSubjectIds": []
}
```

The response contains the workspace revision and up to five possible parents. Every result includes a score, `low`, `medium`, or `high` confidence, its full tree path, and human-readable reasons. The first deterministic model uses shared terms plus exact capability and knowledge-subject evidence already present in workflow steps. It does not require an LLM.

An empty result means Work Graph lacks evidence and refuses to invent a location. The gateway should put that candidate in its future **Unplaced** inbox for a person to resolve. Recommendations are advisory: the gateway still creates an ordinary node using an idempotency key and the returned workspace revision.

## Committed event feed

The gateway can consume tree and workflow changes after they commit:

```http
GET /api/v1/workspaces/{workspaceId}/events?after={cursor}&limit=100
Authorization: Bearer wgsa_…
```

Events are returned oldest first with `nextCursor` and `hasMore`. Each event carries its stable ID, type, aggregate, payload, occurrence time, workspace revision, correlation ID, and actor attribution when available. Save `nextCursor` in the gateway only after the page has been processed durably, then pass it as `after` on the next request. The limit defaults to 100 and may range from 1 to 500.

Delivery is intentionally **at least once**. Reading from the same cursor returns the same committed events, so downstream projection must deduplicate by event ID. An invalid cursor returns `400` instead of silently skipping history. Events are written in the same database transaction as the domain change, so rolled-back changes never appear in the feed.

This feed is pull-based and currently has no retention policy. Webhooks or a message broker may be added later as delivery conveniences, but the durable cursor contract remains the recovery path.

The scopes do not grant access to administration, people, skills, secrets, module policy, priority decisions, or human workflow actions. A token is bound to exactly one workspace and cannot create another workspace.

Use both `work:read` and `work:write` for an Integration Gateway that imports candidates into the native tree. Store provider identities and Jira-to-Work-Graph mappings in the gateway, not in Work Graph. The gateway must use supported HTTP APIs and must never write directly to the database.

Operational rules:

- use TLS outside a trusted local development network;
- keep tokens in a secret manager or protected environment variable, never in source code, URLs, or logs;
- use one identity per independently operated integration;
- revoke unused tokens and replace a token immediately if it may have leaked;
- treat replacement as immediate: the old token stops working as soon as the new one is issued.

Idempotency currently covers native node creation, revision preconditions cover all service-account tree mutations, the first explainable placement endpoint is available, and committed changes can be consumed through a durable cursor. Extending idempotency to the remaining mutations, improving placement from confirmed human feedback, and building the separately deployed Integration Gateway remain subsequent milestones.
