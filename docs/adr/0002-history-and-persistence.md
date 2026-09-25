# ADR 0002: Current State with Immutable Change Events

**Status:** Accepted  
**Date:** 2026-09-22

## Context

Users must inspect earlier revisions and understand changes to trees, claims, catalogs, and diagnostics. A conventional audit log is insufficient, while full event sourcing for every module would add substantial initial complexity.

## Decision

Store normalized current state and append immutable change events for every accepted domain command in the same PostgreSQL transaction.

Each logical command receives a workspace-scoped revision and correlation identifier. An outbox record is written in the same transaction for asynchronous processing.

## Consequences

- Current reads remain straightforward.
- Revision reconstruction is a product invariant.
- The write path must never update tracked state without writing its event.
- Schema migrations must preserve historical interpretation.
- Derived projections can be rebuilt from current state plus retained history according to their documented contract.
