# ADR 0003: Workspace Placement and Read-Only Offline Mode

**Status:** Accepted  
**Date:** 2026-09-22

## Context

The product must scale beyond one server and may later serve multiple geographic regions. Offline access is desirable, but offline editing would require conflict-free or explicit merge semantics for tree and claim mutations.

## Decision

- Pin each workspace to one home region and database shard.
- Replication may provide availability but does not create multiple concurrent write homes.
- Include `workspace_id`, `root_id`, and logical `partition_key` in relevant records from the beginning.
- Support offline access as read-only.
- Defer offline mutations and arbitrary subtree placement.

## Consequences

- Strong transactional invariants remain local to a workspace.
- Different workspaces can scale across shards and regions.
- Very large workspaces may later place independent root trees separately.
- The browser can cache permitted read models without implementing conflict resolution.
