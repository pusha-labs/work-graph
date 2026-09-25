# Initial Product Scope

**Status:** Initial draft  
**Version:** 0.1  
**Working milestone:** Native MVP  
**Reference project:** Build a house

## Objective

Build the smallest standalone version of Work Graph that can be used from an empty workspace to manage a real, evolving project such as building a house.

The Native MVP must prove that a containment tree, dependency graph, temporal history, and explicit knowledge model provide value before external systems are connected.

The MVP is not a Jira importer. Jira and other connectors will be built only after the native data model and inspection tools are usable and understandable.

## Product Hypothesis

A user managing a complex project can make better decisions when they can:

- see how every activity contributes to a larger outcome;
- distinguish containment from dependency;
- inspect people, capabilities, and subject-specific knowledge;
- reconstruct how the plan changed over time;
- identify structural gaps and concentrated knowledge;
- preserve the result as reusable practical knowledge.

## Reference Scenario

The reference user wants to build a house.

They start with an empty workspace, create `Build a house` as a root goal, and progressively add work such as land selection, design, permits, contractors, procurement, construction, inspections, and acceptance.

They record relevant people without requiring each person to create an account. They describe capabilities and knowledge of subjects such as the selected design, local permit process, electrical system, and contractor agreements.

As the project changes, the product preserves earlier revisions. The user can inspect what was created, changed, replaced, or removed and can understand why the current tree has its present form.

## Primary User Journey

1. Start the product using the documented self-hosted setup.
2. Create a workspace.
3. Create the root goal `Build a house`.
4. Add child work nodes.
5. Add dependencies between nodes in different branches.
6. Add people and automated actors to the directory.
7. Invite selected people to create user accounts.
8. Allow each user to inspect and update their own capability and knowledge claims.
9. Use the administrative interface to create capabilities and knowledge subjects and to confirm or add claims.
10. Record that a person has a capability generally or within a particular subject.
11. Attach participants, requirements, artifacts, resources, and decisions to work nodes.
12. Inspect structural and knowledge diagnostics.
13. Open the history view and reconstruct an earlier tree revision.
14. Back up the installation using the documented database procedure.

## Required Capabilities

### Workspace

- Create, rename, and inspect a workspace.
- Store multiple independent root goals.
- Manage workspace membership and basic roles.
- Back up and restore the PostgreSQL database using a documented operational procedure.

The first implementation requires authenticated workspace administrators and members. People represented inside work do not necessarily need login accounts.

Real-time simultaneous editing is not required. Multiple users may work asynchronously, with every change attributed and preserved in history.

### Identity and access

- Authenticate local users.
- Link a user account to one person actor record.
- Support at least `administrator` and `member` workspace roles.
- Allow members to inspect and maintain their own editable profile information.
- Allow administrators to manage workspace catalogs, membership, and confirmed claims.
- Preserve the author and source of every capability or knowledge change.

The MVP must not allow one source to silently overwrite another source's claim.

### Work nodes

- Create a root work node.
- Create a child work node.
- Edit its title, desired outcome, notes, type, and lifecycle status.
- Remove a node from the current tree without destroying history.
- Restore a previously removed node or branch.
- Display requester, participants, requirements, and related subjects.

The product must prevent containment cycles.

### Tree view

- Expand and collapse branches.
- Select a node and inspect details without losing tree context.
- Create, rename, and remove nodes.
- Search for a node by text.
- Filter by status, type, participant, capability, or subject.
- Indicate nodes with unresolved diagnostics.

The first tree view may prioritize clarity and correctness over large-scale visualization performance.

### Dependency graph

- Create typed dependencies between work nodes.
- Inspect incoming and outgoing dependencies from a selected node.
- Detect direct or indirect dependency cycles.
- Distinguish dependency edges visually from containment.

A complete free-form graph canvas is not required if node-level dependency inspection is sufficient for the first usable release.

### People and actors

- Create a person or automated actor record.
- Store a display name and optional notes.
- Associate an actor with work as requester, participant, executor, reviewer, or approver.
- Represent people who do not have product accounts.
- Link an invited person to a user account.
- Allow a person to inspect all claims and evidence visible about them.

Employee monitoring, performance scoring, and automatic ranking of people are explicitly outside the MVP.

### Capabilities

- Create a capability.
- Describe a level using a small configurable scale.
- Associate a capability with an actor.
- Attach evidence, confidence, source, and last-confirmed time.
- Require one or more capabilities on a work node.
- Distinguish self-declared, administrator-confirmed, evidence-backed, inferred, imported, stale, disputed, and revoked claims.

### Knowledge subjects

- Create a knowledge subject.
- Assign a type such as product, project, service, system, location, process, equipment, supplier, or custom.
- Associate work nodes, actors, capabilities, artifacts, and decisions with the subject.
- Record that an actor has subject-specific knowledge.

The same general capability may have different subject scopes. Two actors can both know Python while only one has independently verified knowledge of Product A.

### Knowledge evidence

A knowledge record should be able to distinguish:

- self-declared knowledge;
- knowledge confirmed by another person;
- manually recorded evidence;
- evidence inferred from accepted work history;
- stale or unverified knowledge.

Automated inference from activity is not required initially, but the model must preserve provenance for later use.

### Personal profile interface

Each authenticated member must have a personal interface where they can:

- view their general capabilities;
- add and edit self-declared capability claims;
- view subjects they are recorded as knowing;
- add and edit self-declared subject-knowledge claims;
- attach notes or permitted evidence;
- see which claims were confirmed by an administrator or supported by work history;
- see confidence and review dates;
- dispute or request correction of information they cannot edit directly.

The profile must make claim provenance understandable rather than collapsing all sources into one unexplained skill level.

### Administrative interface

Workspace administrators must have an interface where they can:

- create and manage people and automated actors;
- invite and deactivate user accounts;
- create, merge, rename, archive, and organize capabilities;
- create and manage knowledge subjects and their types;
- assign owners and criticality to subjects;
- add or confirm capability and subject-knowledge claims;
- attach evidence and review dates;
- inspect stale, disputed, or unsupported claims;
- inspect knowledge-concentration diagnostics;
- review the history of administrative changes.

Administrative claims must remain attributed to the administrator and visible to the affected member according to workspace policy.

### Diagnostics

The initial diagnostic engine should identify at least:

- containment cycles;
- dependency cycles;
- non-root native nodes without a parent;
- nodes without a requester;
- required capabilities with no known eligible actor;
- critical knowledge subjects with only one sufficiently knowledgeable actor;
- knowledge records that have not been confirmed within a configured period.

Diagnostics must explain the supporting data and must describe knowledge concentration as a risk of the subject or project, not as a negative label on a person.

### History and revisions

The product must preserve structural and semantic changes, including:

- node creation;
- field changes;
- removal and restoration;
- dependency changes;
- actor and requirement changes;
- knowledge-evidence changes.

The user must be able to:

- view a chronological change list;
- identify the actor and time of a change;
- inspect the tree at a selected revision;
- compare a selected revision with the current state;
- restore removed work without erasing intervening history.

The implementation may use an append-only event model, temporal records, or another design, but revision reconstruction is a product requirement rather than an internal logging detail.

### House-building demonstration data

The repository should include a demonstration workspace containing:

- the house-building tree;
- example dependencies;
- several people and professional roles;
- general capabilities;
- house-specific knowledge subjects;
- at least one temporary critical condition;
- at least one knowledge-concentration diagnostic;
- enough history to demonstrate nodes being created, changed, removed, restored, and improved.

The demonstration must not contain real personal or confidential data.

## Deliberately Deferred

The Native MVP does not require:

- Jira, GitHub, or other external connectors;
- automatic tree generation;
- AI-generated recommendations;
- a complete priority algorithm;
- workflow automation;
- scripts, API steps, or AI-agent execution;
- real-time simultaneous editing;
- mobile applications;
- mature risk-adjusted exchange selection, monetary offers, or contracting;
- automatic capability inference;
- template marketplace or public sharing;
- enterprise SSO or complex permissions;
- payroll, contracting, procurement, or financial accounting;
- a complete project-management reporting suite.

## Debugging and Inspection Requirements

Because future connectors will create and update large trees automatically, the Native MVP must provide tools that make imported behavior understandable later.

For any native visible value or relationship, the system should eventually be able to answer:

- where did it come from;
- who or what changed it;
- when did it change;
- what was its previous value;
- was it created by a person, service actor, inference, or confirmation process;
- which revision contains the change.

The first implementation may expose some of this information in a technical inspector rather than a polished end-user interface.

## Connector Readiness

The Native MVP is ready for the first connector when:

1. A user can create and maintain a meaningful tree manually.
2. The tree and dependency views remain understandable after many edits.
3. Every structural change can be traced and reconstructed.
4. People, capabilities, and knowledge subjects can be represented without Jira-specific concepts.
5. Diagnostics produce understandable results from native data.
6. Native history distinguishes human, service-actor, inferred, and confirmed changes without embedding provider identity in work nodes.
7. There is a documented Integration Gateway boundary that accepts idempotent provider-neutral commands without bypassing domain rules and exposes durable committed events for reverse projection.

At that point, a separate Integration Gateway can translate Jira issues, relationships, users, projects, and updates into placement candidates. It stores Jira-to-Work-Graph mappings, sends confirmed native commands, and lets the user watch the tree evolve with eventual consistency. See [ADR 0005](adr/0005-integration-gateway-boundary.md).

## Success Criteria

The Native MVP is successful when:

- it can be deployed by following the repository documentation;
- a user can build the house example from an empty workspace;
- the user can continue editing it over multiple sessions;
- a node can be edited or removed without losing historical reconstruction;
- a dependency conflict is visible and explainable;
- the system detects a realistic knowledge-concentration risk;
- the user can understand the evidence behind that diagnostic;
- the installation can be backed up and restored;
- no external task-management system is required for the product to be useful.

## Decisions Still Required Before Implementation

- the initial application architecture;
- the local identity, invitation, and authentication approach;
- the exact permission matrix for administrators and members;
- the persistence and revision-history model;
- the initial set of work-node lifecycle statuses;
- the capability-level scale;
- the initial knowledge-confidence scale;
- the representation of artifacts and resources in the MVP;
- the backup and restore procedure;
- accessibility and minimum supported screen sizes;
- the open-source license.
