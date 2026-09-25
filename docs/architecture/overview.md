# Architecture Overview

**Status:** Initial decision baseline  
**Version:** 0.1

## Goals

The architecture must support a small self-hosted installation while preserving a path to horizontally partitioned hosted deployments.

It must provide:

- transactional containment-tree invariants;
- reconstructable history;
- attributable native history and evidence;
- asynchronous multi-user access;
- workspace-level regional placement;
- future partitioning by workspace and root tree;
- connectors that cannot bypass domain rules;
- a responsive web UI with read-only offline access later.

## Initial topology

```text
Browser
  │
  ▼
React web application
  │ JSON HTTP API
  ▼
Go modular monolith
  ├── identity
  ├── workspace
  ├── work graph
  ├── history
  ├── directory
  ├── knowledge
  ├── module registry
  └── diagnostics
  │
  ▼
PostgreSQL 18
  ├── current state
  ├── immutable change events
  └── transactional outbox
```

Automated workflow steps are never executed inside the API process. A separate runner consumes durable execution requests and returns authenticated progress and terminal results. Module configuration is schema-driven; module code runs across the isolation boundary described in [ADR 0004](../adr/0004-workflow-modules-and-isolated-execution.md).

The Native MVP runs through Docker Compose. The frontend and API are separate processes, but the backend remains one deployable service with explicit module boundaries.

## Runtime baseline

- Go 1.27
- Node.js 24 LTS
- React 19
- PostgreSQL 18
- Docker Compose

Developer machines do not need matching host runtimes when using the containerized workflow.

## Persistence model

Current state is stored in normalized relational tables. Every accepted command also appends immutable change events in the same database transaction.

The history is not a best-effort log. It is product data used to reconstruct and compare revisions.

The transactional outbox makes committed events available to background projections and future connectors without requiring a message broker in the Native MVP.

## Integration boundary

External providers connect through a separately deployed Integration Gateway rather than through provider-specific tables in the Work Graph core.

```text
External provider
       │
       ▼
Integration Gateway ── commands and placement requests ──▶ Work Graph API
       ▲                                                   │
       └──────── provider-neutral committed events ────────┘
```

The gateway owns provider credentials, raw payloads, external-item identity mappings, synchronization cursors, retries, conflicts, and the inbox for items whose tree placement is unresolved. It converts a provider object into an External Work Candidate and asks Work Graph for explainable parent recommendations. After automatic or human-confirmed placement, it creates an ordinary native work node through an idempotent domain command.

Work Graph owns native tree structure, history, priority, capability matching, and workflow. It records the scoped integration service actor responsible for a command, but it does not store Jira-specific identity or synchronization state on a work node. Connector access is limited to supported APIs and durable events; direct database writes are prohibited.

See [ADR 0005](../adr/0005-integration-gateway-boundary.md) for mapping ownership, reverse projection, and loop-prevention rules.

## Partitioning model

All tenant-owned records carry a `workspace_id`. Tree-owned records additionally carry a `root_id` and a logical `partition_key`.

Initial placement keeps a workspace in one region and one database shard. Future placement can distribute workspaces and, when necessary, root trees. Arbitrary subtrees are not independently sharded in the MVP.

Public identifiers use UUIDv7. Database sequences may be used internally only when they never escape as domain identity.

## Consistency boundaries

Containment edits, history events, and outbox records commit atomically.

Derived diagnostics, search projections, and notifications may be eventually consistent. User-visible provenance must indicate when a derived view is still being refreshed.

## Offline behavior

Offline behavior is read-only. A future browser cache may show previously synchronized work, profiles, and history. Offline writes and conflict resolution are explicitly outside the architecture baseline.

## Growth path

1. One API process and one PostgreSQL instance.
2. Multiple stateless API processes and background workers.
3. Read replicas and external search projections.
4. Workspace routing to multiple database shards.
5. Root-tree placement for exceptional large workspaces.
6. Multi-region hosted control plane with each workspace pinned to a home region.

Each stage should preserve the HTTP contract and domain command semantics.
