# Work Graph

[![CI](https://github.com/pusha-labs/work-graph/actions/workflows/ci.yml/badge.svg)](https://github.com/pusha-labs/work-graph/actions/workflows/ci.yml)

An open-source platform for understanding, prioritizing, executing, and reusing work.

## Status

Work Graph is at the early MVP stage. The repository contains the first runnable foundation for a standalone, self-hosted workspace that can be used from scratch for a real project such as building a house. External connectors, including Jira, will follow after the native model and tools are usable.

No production-ready release is available yet.

## Run locally

The only local prerequisite is Docker with Docker Compose.

```sh
docker compose up --build
```

Open [http://localhost:8088](http://localhost:8088). The first startup creates the PostgreSQL schema automatically. Stop the application with `docker compose down`; its database remains in a Docker volume.

Before storing credentials, copy `.env.example` to `.env` and set `WORK_GRAPH_MASTER_KEY` to the output of `openssl rand -base64 32`. Keep that key backed up outside the database: losing it makes stored secrets unreadable, while changing it requires an explicit key-rotation procedure. The committed Compose file has no fallback encryption key.

### Load demo data

The demo seeder uses the public API to create a house-building workspace with a branching tree, capabilities, entity knowledge, workflow steps, matching work, and a temporary critical branch. It is safe to run repeatedly: if the named demo workspace already exists, it exits without creating a duplicate.

The script requires `curl` and `jq` in addition to the running Docker Compose application.

For a fresh installation, choose a password of at least 10 characters:

```sh
DEMO_PASSWORD='choose-a-demo-password' ./scripts/demo.sh
```

This creates `demo@workgraph.local` as the initial owner when setup is still required. For an installation that already has accounts, provide credentials for an existing owner or administrator:

```sh
DEMO_EMAIL='owner@example.com' DEMO_PASSWORD='your-password' ./scripts/demo.sh
```

Set `WORK_GRAPH_URL` to seed a different deployment and `DEMO_WORKSPACE_NAME` to create a separately named demo. Do not reuse a public demo password for any real account.

Run `make test-demo` to verify the complete demo path in an isolated Compose project. The test starts a clean installation on a Docker-assigned host port, seeds it twice to verify idempotency, signs in through the public API, and checks the expected tree and diagnostic examples before removing its containers and volume.

On the first launch, Work Graph asks you to create the system owner. The owner is linked to existing local workspaces. Authentication uses an HTTP-only, SameSite session cookie; passwords are stored as bcrypt hashes and sessions expire after 30 days.

### Back up and restore

Create a timestamped, checksummed database archive with `./scripts/backup.sh`. Restore only after verifying the target archive, using `RESTORE_CONFIRM=replace-data ./scripts/restore.sh <archive>`. Restoration automatically creates a safety backup of the database it replaces.

The archive intentionally excludes `WORK_GRAPH_MASTER_KEY`. Keep that key separately in protected storage; restored encrypted secrets are unusable without the original key. See the complete [backup and restore guide](docs/operations/backup-and-restore.md), including the isolated `make test-backup` verification procedure.

Workspace owners and administrators can create seven-day, single-use invitation links from the Directory. Invitees create their own password and join as either a member or administrator; administrative directory operations are enforced by the API.

New workspaces include a compact, data-driven getting-started guide in the tree. It tracks the first branch, capability, participant, and knowledge subject from actual workspace state, links to the relevant existing interface, disappears when setup is complete, and can be dismissed per user without changing shared workspace data.

Every member has a workspace profile. People can add and remove their own self-declared skills, while roles and administrator-verified claims remain under administrative control. Claim provenance is retained so the UI can distinguish self-declared, verified, inferred, and imported knowledge.

The Directory also maintains knowledge subjects such as services, projects, systems, and domains. It records their knowledgeable actors and highlights uncovered subjects, single-person knowledge concentration, and distributed knowledge.

Task circles can combine role and skill requirements with knowledge of specific entities. Performer matching and task claiming require one actor to satisfy every declared condition, for example `Engineer + Python + Product A`.

Task circles can contain an ordered sequence of human workflow steps. Each step has its own capability and entity-knowledge requirements, is claimed independently, and unlocks the next step when completed. The requester remains the first participant and the only person who can provide final acceptance.

Human work will be offered through a step-level exchange rather than assigned by employee name. Eligible people propose a duration independently for each step; the initial policy selects the shortest estimate, while preserved execution observations enable later risk-aware selection. The agreed model is documented in [Human-Step Exchange](docs/task-exchange.md).

The Task circle UI presents that sequence as one continuous route: requester, numbered workflow steps, an explicit **Add next step** action, and requester acceptance. Matching details stay available without obscuring the normal start-and-complete workflow.

The next-step composer resolves one intent into either human requirements or an installed workflow module. The first built-in module editors configure HTTP requests and Bash scripts, persist their versioned configuration, and keep execution disabled until an isolated runner is available. Automated modules use schema-driven editors and execute through a separate isolated runner; see [ADR 0004](docs/adr/0004-workflow-modules-and-isolated-execution.md).

Workflow modules are discovered through a database-backed registry. Each registered version provides its search terms, publisher, step type, and declarative configuration schema; the API validates every automated step against an enabled registry entry instead of trusting module identifiers supplied by the browser.

The composer renders module fields directly from that schema, including text, URL, selection, numeric, multiline, and code inputs. The API applies the same schema contract to required fields, primitive types, and allowed options, so adding a differently shaped module no longer requires a Task circle UI change.

Before execution starts, clicking a route card opens its editor. Step names, human matching requirements, and module configuration can be changed; steps can also be moved earlier or later or removed. The API rejects structural edits after work starts and prevents deletion of the final remaining step. Every accepted edit advances the workspace revision and is attributed in activity history.

Each workflow step now has a separate execution-attempt history. Claiming human work starts a numbered attempt, completion records its finish, and requester returns mark the affected successful attempts as returned or superseded. Every attempt preserves a snapshot of the exact step name, type, module version, configuration, and matching requirements that were in force when it started.

The outbound HTTP runner is a separate binary and an opt-in Docker Compose profile. It remains disabled unless both `RUNNER_TOKEN` and an explicit comma-separated `HTTP_RUNNER_ALLOWED_HOSTS` list are supplied. It accepts only allowlisted public HTTP(S) destinations, revalidates redirects and DNS results, blocks private/local/link-local networks, caps redirects, response size, and runtime, and reports structured results back through authenticated runner endpoints. Start it explicitly with `RUNNER_TOKEN=... HTTP_RUNNER_ALLOWED_HOSTS=api.example.com docker compose --profile automation up -d runner`; never reuse the example hostname or commit a real token. The requester can cancel an active execution or retry a failed, timed-out, or cancelled step. Cancellation is durable immediately, rejects late results, and is observed by the runner so its in-flight request context is interrupted.

Workspace owners and administrators can create, replace, and disable encrypted secret records. Secret values are protected with AES-256-GCM, bound to their workspace as authenticated data, and never returned by the ordinary API after creation. HTTP steps store only a secret ID and can inject its value as a bearer token or explicitly named header. The authenticated runner receives the decrypted value only when it leases that exact step, sends it only to an allowlisted destination, and redacts direct echoes from captured response bodies. A referenced secret cannot be disabled while unfinished workflow steps still depend on it.

Module availability and permissions are workspace-specific. Administrators must explicitly trust the publisher of an exact module version before enabling it. They can then configure exact destination hostnames for HTTP and grant or withhold secret access independently. Modules referenced by unfinished routes cannot be disabled. The API enforces the policy when a step is created, edited, and leased; the runner's environment allowlist remains an additional deployment-level ceiling.

My work gives each person a cross-tree queue of the steps they have already claimed and the ready steps for which they satisfy every requirement. Active work, temporary criticality, and explicit cross-root priority decisions influence the order without introducing a manually maintained priority number. Queue items navigate directly to the selected node in Tree & History, where actions remain beside the full branch context.

The current tree overlays that same personal order directly on actionable nodes as `1`, `2`, `3`, and so on. The numbers are a per-actor projection—not mutable task fields—and identify both work already in progress and the next matching steps to start.

The tree also exposes explainable structural and knowledge diagnostics. It marks open work without a requester, ready human steps with no eligible performer, tasks awaiting acceptance while descendants remain open, knowledge subjects with no known holder, and subjects known by only one person. A compact attention panel shows the evidence behind each result and navigates to the affected task or knowledge administration. Fresh demo workspaces intentionally include unresolved performer and knowledge-concentration diagnostics so the behavior is immediately visible.

After the final step, the requester sees the task in the same personal order as `Needs your review`. They can accept it or return any completed step with a required explanation; that step becomes ready again, later steps wait, and the decision remains visible in Activity.

The containment tree also expresses required work. Creating a child task or branch decomposes its parent's result; the parent can progress in parallel, but cannot be closed until every descendant is closed. Open child work is visible in the Task circle.

The Priority Inbox detects when the same eligible performer has actionable work under different root goals. Workspace owners and administrators can resolve the conflict by comparing goals or, when context is insufficient, requester trust. The latest explicit decision orders Current work and preserves its reasoning without introducing an arbitrary priority number.

Requesters and workspace administrators can also mark a specific branch temporarily critical with a reason and deadline. Active criticality propagates visually through every ancestor to the root goal, moves affected work forward on the board, expires automatically, and can be revoked explicitly.

Tree & History includes a unified Activity timeline for node revisions, requirements, priority decisions, and criticality creation or revocation. Tree events remain linked to read-only historical snapshots, while management events retain their actor and explanation as audit context.

New structural and workflow events record the responsible workspace actor. Directory, capability, knowledge, profile, and invitation changes produce account-attributed audit events. Legacy events created before authorship support intentionally remain attributed to `System` rather than being rewritten without evidence.

The current UI includes a current-work board, a branching tree with historical snapshots, Task circles, a Priority Inbox, personal profiles, and an administrative directory. It remains an early MVP and is not production-ready.

## Why

Traditional task trackers record what needs to be done, but they often fail to explain why work exists, how it contributes to larger goals, and what should be done next.

Work Graph explores a different model:

- every native work item belongs to a larger purpose;
- priority is derived from context rather than stored as an isolated label;
- human and automated work can participate in one transparent execution flow;
- completed work can become reusable, versioned operational knowledge;
- existing systems such as Jira can remain the system of record.

## Project vision

The initial project vision is available in [`docs/vision.md`](docs/vision.md). The shared domain language is defined in [`docs/concepts.md`](docs/concepts.md), the first prioritization model is described in [`docs/priority-model.md`](docs/priority-model.md), deferred and near-term work is recorded in [`docs/roadmap.md`](docs/roadmap.md), the native MVP boundary is defined in [`docs/initial-product-scope.md`](docs/initial-product-scope.md), and its initial UI surfaces are described in [`docs/ui-surfaces.md`](docs/ui-surfaces.md). The separate Integration Gateway boundary is recorded in [`ADR 0005`](docs/adr/0005-integration-gateway-boundary.md), the core authentication surface is documented in [`docs/api-access.md`](docs/api-access.md), and the separately deployed gateway skeleton is described in [`docs/integration-gateway.md`](docs/integration-gateway.md).

An end-to-end non-corporate example is available in [`docs/examples/building-a-house.md`](docs/examples/building-a-house.md).

## Current direction

The first user journey is intentionally narrow and independent of external systems:

1. Create a workspace and a root goal such as `Build a house`.
2. Build and edit a containment tree.
3. Add dependencies, people, capabilities, and knowledge subjects.
4. Inspect the current tree and its complete change history.
5. Detect structural and knowledge-concentration risks.
6. Use the workspace for real work before importing any external data.

## Open source

The project is licensed under the [GNU Affero General Public License v3.0 only](LICENSE). Copyright 2026 Work Graph contributors.

The project is intended to have a practically useful open-source core and support self-hosted deployment. The exact boundary between open and future commercial capabilities has not yet been selected.

## Contributing

The project is not ready for broad feature contributions yet, but focused fixes, tests, documentation, use cases, and feedback are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before proposing a change. Every push and pull request is checked by CI using the same containerized backend, frontend, operational, backup/restore, and demo-deployment verification available locally.
