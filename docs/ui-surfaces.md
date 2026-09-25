# Initial UI Surfaces

**Status:** Initial draft  
**Version:** 0.1  
**Scope:** Native MVP

## Objective

The Native MVP needs interfaces for three different kinds of work:

1. Structuring and debugging the work graph.
2. Allowing each person to understand and maintain their own capability and knowledge profile.
3. Allowing administrators to manage the shared catalogs, evidence, and risk diagnostics.

These surfaces operate on the same domain model but must not be compressed into one overloaded screen.

## Surface 1: Work Graph Explorer

The Work Graph Explorer is the primary interface for goals, work nodes, dependencies, and history.

### Tree view

The default view shows the containment tree.

```text
Build a house
├── Select and purchase land
├── Design the house
├── Obtain permits
├── Prepare construction
└── Construct the house
```

The user can create, select, edit, remove, restore, search, and filter work while seeing diagnostics and active critical signals.

### Node inspector

Selecting a node opens an inspector without losing the surrounding tree. It shows desired outcome, type, status, requester, participants, capabilities, subjects, dependencies, artifacts, resources, decisions, provenance, changes, and diagnostics.

### Dependency inspection

The user can inspect incoming and outgoing typed dependencies. A complete free-form graph canvas can be deferred if node-level inspection is sufficient. Containment and dependency edges must remain visually distinct.

### History and diff

The history view reconstructs a selected revision, compares revisions, and identifies created, changed, removed, and restored nodes with their author, source, and reason.

## Surface 2: My Profile

Every authenticated member has a personal profile focused on transparency and self-service.

### My capabilities

The user can see general capabilities, self-declared levels, administrator-confirmed levels, evidence-backed or inferred claims, last-confirmed dates, and stale, disputed, or revoked claims.

```text
Python development
├── self-assessment: advanced
├── administrator confirmation: intermediate
├── evidence: 12 accepted work items
├── last demonstrated: 2026-08-14
└── status: review suggested
```

The UI must not silently collapse conflicting claims into one authoritative-looking value.

### My subject knowledge

The user can see knowledge scoped to products, services, systems, projects, locations, processes, or other subjects.

```text
Product A
├── Python implementation: advanced
├── deployment process: working knowledge
└── incident response: basic

Product B
└── no confirmed knowledge
```

This distinguishes general skill from contextual familiarity.

### Editing and correction

The user can add or edit self-declared claims, attach allowed evidence, request confirmation, dispute other-source claims, and inspect who added or confirmed information.

Users cannot rewrite administrator confirmations, imported facts, or historical evidence as if they authored them.

### Privacy and trust

The personal UI shows who can see data, where each claim came from, whether it is used for matching or diagnostics, how to request correction, and whether it is stale or uncertain.

Hidden employee scoring is outside the product vision.

## Surface 3: Administration

The administrative interface manages the shared vocabulary and governance of a workspace.

### People and accounts

Administrators can create person and automated-actor records, invite users, link accounts and people, activate or deactivate access, assign basic roles, and inspect administrative history.

A person can exist without an account.

### Capability catalog

Administrators can create, describe, organize, merge, and archive capabilities; define level scales; and inspect where capabilities are required or claimed.

Capabilities should be reusable vocabulary, not duplicated free-text labels per person.

### Knowledge-subject catalog

Administrators can manage subjects such as Product A, Product B, a payment service, ClickHouse cluster, house electrical system, local permit process, supplier, customer, or custom type.

For each subject, administrators can manage type, description, owner, criticality, lifecycle state, related work, relevant capabilities, and knowledge coverage.

### Claims and evidence

Administrators can add an administrator-sourced claim, confirm or decline requested confirmation, attach evidence, set review dates, flag disputes, and compare self-declared and supported information.

Every action remains attributed. An administrative edit must not masquerade as a self-assessment.

### Risk diagnostics

The administrator can inspect subject-level risks:

```text
Product A: high knowledge-concentration risk

Evidence:
- one actor has independently confirmed operational knowledge;
- no second actor can complete the deployment process;
- the subject is marked business-critical;
- no active knowledge-transfer work exists.
```

The UI describes a resilience risk of Product A, not a negative judgment of the knowledgeable person.

## Shared Interaction Principles

- Provenance is visible.
- Changes to trees, catalogs, claims, and criticality are attributable and inspectable.
- Destructive operations are normally recoverable.
- Diagnostics link to the facts that produced them.
- The initial web interface is responsive, but a dedicated mobile application is deferred.

## Initial Navigation

```text
Workspace
├── Work
│   ├── Tree
│   ├── Dependencies
│   └── History
├── Directory
│   ├── People
│   ├── Capabilities
│   └── Subjects
├── Diagnostics
└── Administration

Personal menu
└── My Profile
    ├── Capabilities
    ├── Subject Knowledge
    └── Evidence and Review
```

Navigation labels are provisional and should be tested before becoming permanent domain terminology.

## MVP Permission Summary

| Action | Member | Administrator |
| --- | --- | --- |
| View permitted work trees | Yes | Yes |
| Edit permitted work | Policy-dependent | Yes |
| View own claims | Yes | Yes |
| Edit own self-declared claims | Yes | Yes, for their own profile |
| Edit another person's self-declaration | No | No |
| Add an administrator claim | No | Yes |
| Create capabilities | No | Yes |
| Create knowledge subjects | No | Yes |
| Manage accounts and roles | No | Yes |
| Inspect workspace diagnostics | Policy-dependent | Yes |
| View attributed history | Policy-dependent | Yes |

The final permission model remains an implementation decision. This table defines the minimum distinction required for the Native MVP.

## Open Questions

- Can members propose capabilities or subjects for approval?
- Which evidence can contain private attachments?
- Should members see every administrator note or only claims that affect them?
- Who confirms an administrator's own claims?
- Can subject owners manage claims without full administrator access?
- How are disputes resolved and escalated?
- Which diagnostics should ordinary members see?
- What information is visible across workspaces?
