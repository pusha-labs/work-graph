# Work Graph Roadmap

This document records important product areas that must not be lost when implementation order changes. It is directional rather than a release commitment.

## Deferred but fundamental: explainable priority resolution

Priority resolution is one of the defining capabilities of Work Graph. It is intentionally deferred while the native work model and editing tools mature, because a credible implementation requires more than a sorting formula.

The future mechanism must:

- derive personal work order from the containment tree, workflow readiness, capability matching, temporary criticality, and explicit human decisions;
- identify the correct decision owner from the structural or resource intersection of competing work;
- ask contextual questions instead of requiring arbitrary numeric priorities;
- support escalation from a goal comparison to requester trust or another relevant decision basis when the first question cannot be answered;
- represent unresolved comparisons honestly instead of inventing a total order;
- explain why an item received its position and which decisions affected it;
- preserve decisions, their authors, their scope, and their history;
- recalculate affected personal tree projections when context changes;
- avoid treating a deterministic UI fallback, such as title or creation order, as semantic priority.

The current numbered tree is an early projection. It uses active work, criticality, recorded cross-root decisions, readiness, and matching, but it is not the final priority engine. Stable fallback ordering is presentation only.

## Near-term native MVP

Before returning to the full priority engine, the project should strengthen the tools needed to create and operate native work:

1. Build the first human-step exchange slice: bids with proposed durations, deterministic shortest-estimate selection, step assignment, and preserved decision history.
2. Record agreed and actual elapsed duration per human execution attempt, then maintain separate estimation-bias, stability, exceptional-overrun, and observation-count statistics. Zero observations must remain visibly unknown.
3. Edit unstarted human workflow steps: rename, reorder, change requirements, and remove.
4. Extend the new per-attempt execution history with automated-run logs, structured outputs, failures, timeouts, and cancellation.
5. Build on manual, requester-controlled HTTP retries and runner-side cancellation with secret references and per-module permission administration. Every retry must preserve the failed attempt and create a new numbered attempt; cancelled attempts must reject late results.
6. Extend the workspace module policy UI with installation and version upgrades. Enable/disable controls, exact-version publisher trust, exact HTTP host permissions, secret-access review, and referenced-module protection are now present.
7. Add Bash only after the isolated runner boundary has been implemented and verified.
8. Continue expanding tree diagnostics. Live checks now cover missing requesters, ready human steps without an eligible performer, requester review blocked by open descendants, uncovered knowledge subjects, and single-person knowledge concentration; deeper structural checks and configurable criticality remain.

The exchange model and its implementation order are defined in [Human-Step Exchange](task-exchange.md).

The self-hosted data-safety baseline is now present: checksummed PostgreSQL archives, explicit and guarded restore, automatic pre-restore safety backup, encryption-key guidance, and an isolated end-to-end restore verification test.

The first-run usability baseline now includes a dismissible, per-user guide derived from real workspace state rather than a separate tutorial database.

The public-contributor baseline now includes automated push and pull-request verification for backend tests, the production web build, operational scripts, and isolated backup/restore, plus a contribution guide tied to the project's vision and ADRs.

The demo path now has an isolated deployment smoke-test that verifies clean startup, repeatable seeding, authentication, the house tree, intentional diagnostic examples, and the service-token lifecycle on every push and pull request.

The first provider-neutral integration foundation is now present: workspace-scoped automation identities, one-time API tokens stored only as digests, read/write scopes, immediate rotation and revocation, administrative audit events, attribution of native tree changes to the service actor, atomically idempotent native-node creation, optimistic workspace-revision protection for every service tree mutation, deterministic placement recommendations with visible evidence and honest no-match results, and an at-least-once committed event feed with durable cursors.

The separately deployed Integration Gateway skeleton is now present with its own PostgreSQL database, an authenticated admin boundary, a mock provider adapter, durable candidates and mappings, and an Unplaced/suggested inbox requiring human confirmation before creation. The inbox includes a complete visual tree picker and deep links that reveal the chosen branch or created task in the native tree. Confirmed placements now have durable exponential-backoff retries, database leases for multi-replica workers, visible attempt state, and a dead-letter state that requires human recovery after the retry budget is exhausted. The gateway consumes committed Work Graph events with an atomic durable cursor, deduplicates them into a mapped outbound journal, and suppresses revisions caused by its own placement command. A provider-neutral installation registry now stores adapter configuration and AES-256-GCM encrypted credentials without returning secret material through its API or UI. Jira Cloud installations can complete OAuth 2.0 authorization with expiring, one-use state validation and configured-site verification; access and refresh tokens remain encrypted. Bounded, cursor-paginated, read-only JQL imports run manually or on durable five-minute schedules. Multi-replica leases, per-installation locks, automatic access-token refresh, rotating refresh-token replacement, failure backoff, visible synchronization health, and unchanged-version suppression are present.

## Later integrations

- extend idempotency across the remaining mutations and retain confirmed placement feedback;
- narrowly scoped, opt-in Jira reverse projection;
- reusable branch export and import;
- cross-server branch sharing;
- template extraction and versioning;
- notification channels and chat integrations.

The integration boundary and delivery order are defined in [ADR 0005](adr/0005-integration-gateway-boundary.md). Provider identity and synchronization state belong to the gateway, not to native Work Graph nodes.
