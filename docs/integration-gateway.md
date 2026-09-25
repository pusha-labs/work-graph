# Integration Gateway

The Integration Gateway is a separately deployed service with its own PostgreSQL database and security boundary. It translates provider objects into provider-neutral candidates, owns external-to-native mappings, and talks to Work Graph only through scoped HTTP APIs.

The initial skeleton includes:

- a mock provider endpoint for normalized candidate ingestion;
- durable upsert by provider, installation, and external item identity;
- calls to Work Graph placement recommendations;
- an authenticated placement inbox at port `8090`;
- explicit human confirmation before native node creation;
- idempotent Work Graph commands with workspace revision protection;
- durable external-to-native mapping after successful creation;
- durable placement retries with exponential backoff and a dead-letter state after five failed attempts;
- at-least-once consumption of committed Work Graph events with an atomic durable cursor;
- a deduplicated outbound journal for mapped native nodes, including suppression of gateway-originated revisions.

## Configuration

The gateway refuses all protected endpoints until `GATEWAY_ADMIN_TOKEN` is configured. The inbox uses HTTP Basic authentication: any username is accepted and the admin token is the password. API clients use the same value as a Bearer token.

The Work Graph service token must be created under **Administration → API access** with `work:read` and `work:write` scopes. Keep both tokens out of source control.

Required production settings:

| Variable | Purpose |
| --- | --- |
| `GATEWAY_ADMIN_TOKEN` | Protects the gateway UI and API |
| `GATEWAY_MASTER_KEY` | Base64-encoded 32-byte key that encrypts provider credentials at rest |
| `GATEWAY_PUBLIC_URL` | Browser-visible gateway origin used to build OAuth callbacks; HTTPS is required outside localhost |
| `GATEWAY_DATABASE_URL` | Gateway-owned PostgreSQL database |
| `WORK_GRAPH_SERVICE_TOKEN` | Scoped credential used only for core API calls |
| `WORK_GRAPH_URL` | Internal URL of the Work Graph API |
| `WORK_GRAPH_PUBLIC_URL` | Browser-visible Work Graph URL used by “Show in tree” links |
| `WORK_GRAPH_WORKSPACE_ID` | Target workspace; optional when the token can see only one |
| `GATEWAY_INSTALLATION_KEY` | Stable identity of this provider installation |

With Docker Compose, the UI is available at `http://localhost:8090`. Leaving either security token empty is acceptable for diagnostics, but ingestion and placement remain unavailable.

## Mock ingestion

```http
POST /api/v1/mock/candidates
Authorization: Bearer <gateway-admin-token>
Content-Type: application/json

{
  "externalId": "MOCK-1",
  "externalVersion": "1",
  "title": "Prepare construction site utilities",
  "description": "The ground and utilities are ready",
  "rawPayload": {}
}
```

The mock adapter exists to verify the provider boundary before Jira libraries, credentials, or payloads are introduced. A candidate with evidence-backed recommendations becomes `suggested`; one without evidence remains `unplaced`. The inbox shows those recommendations first, but also provides the complete native tree for manual placement. Its links open the exact parent or newly created task in the Work Graph tree. Only a confirmed parent creates a native Work Graph node.

If Work Graph temporarily rejects or cannot complete a confirmed placement, the gateway keeps the chosen parent and schedules another attempt. Delays grow from 15 seconds up to 15 minutes. Multiple gateway replicas claim due work with a database lease, so an attempt is processed by one worker at a time. After five failed attempts, the candidate enters `dead_letter`; the inbox preserves the last error and asks an administrator to select a branch again or explicitly retry. Re-ingesting a newer external version also clears an unresolved delivery failure so it can be reviewed with fresh recommendations.

The gateway also polls the committed Work Graph event feed. It stores its cursor in the same transaction that inserts deduplicated outbound journal records, so a restart can replay safely without losing or duplicating a projected change. Only events for native nodes with an external mapping enter the journal. A revision at or below the mapping's last gateway-written revision is marked `suppressed`; a later native change is marked `pending` for a future provider adapter. The admin inbox exposes both states for inspection. This is the first loop-prevention layer; an actual Jira adapter must additionally retain provider versions and field fingerprints when it delivers a pending event.

## Provider installations

The gateway has a provider-neutral installation registry. An installation records a provider key, human-readable name, HTTPS base URL, authentication type, non-secret configuration, lifecycle status, and credential version. Credentials are JSON internally so adapters can define their own fields, but they are encrypted with AES-256-GCM before they reach PostgreSQL and bound cryptographically to the installation ID. Listing APIs and the admin UI expose only `hasCredentials`, version, and update time—not ciphertext, nonce, or plaintext.

`GATEWAY_MASTER_KEY` must decode to exactly 32 random bytes. Losing it makes existing provider credentials unrecoverable; changing it requires an explicit future rotation procedure. Disabling an installation immediately erases its encrypted credentials.

The Jira Cloud form first creates a secure `draft`. An administrator can then choose **Connect with Atlassian**. The gateway uses the OAuth 2.0 authorization-code flow with read-only Jira access and offline refresh access. Authorization state is random, stored only as a SHA-256 digest, expires after ten minutes, and is consumed exactly once. The callback exchanges the code at Atlassian's fixed token endpoint, retrieves the token's accessible resources, and requires an exact match with the configured Jira site before activating the installation. Access and refresh tokens are stored only in the encrypted credential document.

For local development the callback is `http://localhost:8090/api/v1/oauth/jira/callback`. A deployed gateway must set `GATEWAY_PUBLIC_URL` to its HTTPS public origin and register the resulting callback URL in the Atlassian developer console. The current form accepts development client credentials; before a hosted or Marketplace distribution, Work Graph must use its own centrally registered distributable Atlassian app rather than asking each customer to create one.

Connected Jira installations expose a manual **Import recent issues** action. It calls Jira's enhanced JQL search API through the verified cloud ID, requests only issue fields needed by the placement workflow, follows cursor pagination, and upserts results into the existing placement inbox. Atlassian Document Format descriptions are reduced to plain text for recommendations. The default query is `ORDER BY updated DESC`; a connection may define a narrower JQL filter. Imports read 25 issues by default and are hard-capped at 100 per run. Existing native mappings are preserved when the same Jira issue is seen again.

Before a Jira request, an expired access token is refreshed using the encrypted refresh token. Atlassian's rotated refresh token replaces the previous encrypted credential document; token material is never included in API responses or UI.

Each connected installation receives a durable PostgreSQL-backed schedule, currently every five minutes. Workers claim due schedules with `FOR UPDATE SKIP LOCKED` and a two-minute lease, so multiple gateway replicas can cooperate without taking the same scheduled run. An installation-level advisory lock also serializes manual and automatic runs, protecting rotating refresh tokens. Failed runs use exponential backoff capped at one hour; the admin UI exposes the last completion, next run, failure count, and a sanitized error. Disabling an installation disables its schedule and clears any lease. Unchanged provider versions bypass placement recommendations and database rewrites.

Synchronization remains strictly read-only. Each cycle freezes an upper time boundary, reads changes in ascending update order, and persists Jira's `nextPageToken` between bounded batches. The durable high-water cursor advances only when Jira reports the last page. Consecutive cycles overlap by two minutes because JQL timestamps have minute precision; external versions make that overlap idempotent. Large installations therefore drain safely over multiple worker iterations instead of losing work outside a fixed recent-result window. The last 50 sanitized run records are available from `GET /api/v1/installations/{id}/sync-runs`.

The connection card keeps administrative controls inside a collapsed section. Administrators can change the JQL filter, one-to-1,440-minute interval, and one-to-100 issue batch size; the gateway validates the proposed JQL against Jira before committing it. Changing the query resets the high-water cursor so matching work is re-evaluated without breaking existing mappings. Synchronization can be paused or resumed, recent sanitized runs are visible in the UI, and an explicitly confirmed reset can restart a complete scan.

Narrowly scoped reverse projection remains a future increment.
