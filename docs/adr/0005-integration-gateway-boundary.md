# ADR 0005: Integration Gateway Boundary

**Status:** Accepted  
**Date:** 2026-09-23

## Context

Work Graph must integrate with Jira and other work-management systems without becoming a provider-specific synchronization layer. External items arrive without reliable placement in the Work Graph containment tree, while Work Graph derives native information such as priority, capability requirements, and recommended responsibility that may later be projected back to the provider.

A connector therefore needs durable identity mapping, retries, conflict handling, an interface for unresolved placement, and protection against synchronization loops. Those concerns have a different lifecycle and scaling profile from the Work Graph domain.

## Decision

### Separate service boundary

Integrations run in a separate **Integration Gateway** service and deployment. The gateway may live in a separate repository, such as `work-graph-integrations`.

```text
Jira / another provider
        │ events and polling
        ▼
Integration Gateway
  ├── provider adapters
  ├── external-item mappings
  ├── synchronization state
  ├── placement inbox
  └── outbound policies
        │ stable commands, queries, and events
        ▼
Work Graph
  ├── native work nodes and tree
  ├── placement recommendations
  ├── priority and matching
  └── domain history and outbox
```

The gateway uses authenticated public domain APIs and events. It must never write directly to the Work Graph database or bypass tree, authorization, history, or workflow rules.

Work Graph remains fully useful without the gateway. Provider availability and gateway availability must not sit on the synchronous request path for ordinary native work.

### External Work Candidate

Provider adapters normalize incoming objects into a provider-neutral **External Work Candidate**. It may contain a title, description, type, timestamps, labels, opaque actor references, grouping and link signals, and other evidence useful for placement.

The candidate is an integration message, not a new provider-shaped Work Graph entity. Provider identifiers, raw fields, synchronization cursors, and ownership rules remain in the gateway.

### Placement protocol

For each new or materially changed candidate, the gateway asks Work Graph for placement recommendations. A recommendation contains a possible parent work node, confidence, and human-readable reasons.

Gateway policy determines the next action:

- high confidence may create the node automatically when an administrator has enabled that policy;
- medium confidence presents ranked suggestions;
- low confidence or no valid parent places the candidate in an **Unplaced** inbox.

The integration UI lets a person inspect the candidate, choose a parent in the native tree, and confirm creation. Confirmed choices are retained as placement feedback so future recommendations can improve. Consequential structural changes remain subject to workspace policy and must not occur silently by default.

### Native creation and identity mapping

Once placement is confirmed, the gateway sends an idempotent create command to Work Graph. Work Graph creates an ordinary native work node and attributes the command to a scoped integration service actor.

The gateway owns the mapping between provider and native identities. At minimum it stores:

- provider installation or tenant identity;
- provider item identity and version;
- Work Graph workspace and work-node identity;
- last observed native revision;
- field ownership and synchronization policy;
- field fingerprints needed for echo suppression and conflict detection.

Work nodes do not receive a Jira-specific `source`, issue key, synchronization status, or raw provider payload. The Work Graph audit trail may state that a command was performed by the `Jira Connector` service actor without making Jira part of the domain model.

### Reverse projection

The gateway consumes committed Work Graph events from a supported event or outbox API. It may project selected results back to Jira, for example an assignee suggestion, comment, label, custom field, or status transition.

Every outbound action requires an explicit administrator policy defining which field is owned by which system and whether the action is advisory or authoritative. The initial connector is read-only toward Jira; write-back is added incrementally and remains opt-in.

### Delivery, conflicts, and loop prevention

Synchronization is asynchronous and at-least-once. Both sides must tolerate duplicate delivery.

- Gateway commands carry stable idempotency keys.
- Work Graph commands support optimistic revision preconditions where concurrent edits matter.
- The gateway records provider versions, native revisions, operation identifiers, and field fingerprints.
- Changes caused by a gateway operation are recognized and suppressed when echoed back.
- Conflicting edits are surfaced for policy-based or human resolution instead of silently overwriting either side.
- Durable retries and a dead-letter state preserve failures for inspection.

### Required Work Graph contracts

Before the first production connector, Work Graph must provide:

- scoped service identities and permissions;
- stable commands and queries for native work;
- idempotent mutation handling;
- optimistic revision checks;
- a durable, ordered event or outbox consumption contract;
- placement recommendations with confidence and explanations.

These contracts are provider-neutral.

## Consequences

- Work Graph stays a standalone product rather than becoming a Jira proxy.
- Jira, GitHub, Linear, and future adapters can share gateway infrastructure without contaminating the core model.
- The gateway database and mapping records become operationally critical and require backup, migration, and observability.
- Users may briefly see eventual inconsistency between systems.
- Uncertain placement becomes visible work in a dedicated inbox instead of creating misleading tree structure.
- Running another service adds deployment complexity, but failures are isolated from native Work Graph operation.

## Delivery sequence

1. Stabilize the provider-neutral Work Graph command, placement, and event contracts.
2. Build a gateway skeleton and mock provider adapter.
3. Add Jira read-only ingestion, durable mappings, and the Unplaced inbox.
4. Learn from confirmed placement decisions and improve recommendations.
5. Add narrowly scoped, opt-in Jira write-back policies with conflict and loop protection.
