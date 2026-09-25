# Core Concepts

**Status:** Initial draft  
**Version:** 0.1  
**Purpose:** Establish a shared language for product, design, and architecture discussions.

This document defines conceptual distinctions. It is not yet a database schema or API specification.

## Model Overview

Work Graph represents work as a set of independent purpose trees connected by additional relationships.

A tree begins with a meaningful desired outcome. It may describe personal, shared, or organizational work. The model does not require a company, department, project portfolio, or predefined hierarchy.

Examples of valid roots include:

- build a house;
- learn Dutch;
- launch a product;
- open a data center;
- improve the reliability of a service;
- organize a community event.

A workspace can contain multiple roots. Together, these roots form a forest. Additional dependency and reference relationships can connect nodes within or across trees without changing their primary place in the containment structure.

## Workspace

A **Workspace** is a security, storage, configuration, and collaboration boundary.

A workspace may belong to one person or be shared by a family, team, community, or organization. It can contain multiple independent root work nodes.

A workspace is not automatically a visible parent of those roots. Any technical workspace root used internally must not affect user-facing purpose, progress, or priority calculations.

## Work Node

A **Work Node** is the universal unit used to represent intentional work or a desired outcome.

A work node may represent a goal, project, phase, task, milestone, decision activity, purchase activity, inspection, approval, or another user-defined concept.

A work node can contain:

- a desired outcome;
- a primary parent;
- child work nodes;
- dependencies and other relationships;
- a requester;
- participants;
- capability requirements;
- a workflow;
- priority inputs and explanations;
- related artifacts, resources, decisions, and events;
- provenance from a template or external system.

The domain model should not require rigid levels such as `Goal → Project → Epic → Task`. Types add meaning and behavior, but do not permanently determine depth.

## Root Work Node

A **Root Work Node** is a work node with no user-visible containment parent.

It represents a meaningful top-level desired outcome within a workspace. Root nodes are allowed and expected. Non-root native work nodes must have exactly one primary containment parent.

The importance of a root may be specified directly or compared with other roots in the same priority context.

## Work Node Type

A **Work Node Type** describes the semantic role and optional behavior of a work node.

The product may provide built-in types such as:

- goal;
- project;
- phase;
- task;
- milestone;
- approval;
- purchase;
- inspection.

Workspaces may define additional types. A type may define fields, validation rules, allowed relationships, workflow defaults, or presentation. It must not force every workspace to adopt the same hierarchy.

## Desired Outcome

A **Desired Outcome** describes the result that should be true when a work node is successfully accepted.

It is different from an activity description. “Contact three contractors” describes an action; “a qualified contractor has been selected under agreed terms” describes an outcome.

Outcome-oriented descriptions make acceptance, reuse, and prioritization more reliable.

## Containment Tree

The **Containment Tree** answers:

> What larger outcome is this work part of?

Every non-root native work node has exactly one primary containment parent. This creates a clear chain of purpose from a detailed activity to a root goal.

Containment is not the same as dependency, chronological order, ownership, or tagging.

Moving a node in the containment tree changes the meaning and priority context of that node. It is therefore a consequential operation that should be recorded and, when recommended automatically, confirmed by a human.

In the MVP, decomposition also expresses required work: a child is a necessary part of achieving its parent's result. A parent may be worked on while its children progress, but it cannot be closed while any descendant remains open. Creating a separate dependency edge is therefore unnecessary for this core case.

## Work Forest

A **Work Forest** is the collection of independent containment trees inside a workspace.

Users are not required to invent an artificial universal life goal or company goal to connect unrelated roots. Cross-root comparison is performed through an explicit priority context, not by adding a misleading parent.

## Relationship and Dependency Graph

A **Relationship** connects two entities without changing their primary containment positions.

A **Dependency** is a relationship that affects execution or readiness. Common dependency meanings include:

- blocks;
- is blocked by;
- enables;
- requires;
- duplicates;
- relates to;
- is informed by.

A node can have many relationships. Relationships may connect different branches or different root trees.

The first implementation should support a small, explicit set of relationship semantics rather than one ambiguous generic link.

## Work, Artifact, Resource, Decision, and Event

Not everything related to work is itself a work node.

### Work Node

Represents an intention, activity, or desired outcome.

Example: “Select and purchase windows.”

### Artifact

An **Artifact** is information or a deliverable created, received, or used during work.

Examples include a quotation, architectural drawing, contract, invoice, checklist, report, photograph, or source-code change.

### Resource

A **Resource** is something consumed, reserved, purchased, or otherwise required by work.

Examples include money, materials, equipment, space, time allocation, or a service subscription.

### Decision

A **Decision** records a choice, its alternatives, its rationale, its author, and its consequences.

Example: selecting one contractor after comparing price, availability, and warranty terms.

### Event

An **Event** records something that happened at a particular time.

Examples include a delivery, approval, failure, status transition, assignment, external synchronization, or workflow completion.

These entities may be attached to work nodes without appearing as children in the containment tree.

## Person and Automated Actor

An **Actor** is an entity capable of participating in work.

An actor may be:

- a person;
- a team or candidate group;
- an external service;
- an API endpoint;
- a script or job worker;
- a software module;
- an AI agent.

Human and automated actors can participate in the same workflow, but their identity, permissions, audit requirements, and failure behavior may differ.

### Workflow Module

A **Workflow Module** defines a type of automated step. It contributes searchable terms, a versioned configuration schema, requested permissions, input and output contracts, and an execution artifact. Bash and outbound HTTP are modules rather than hard-coded exceptions.

A **Module Execution** is one durable attempt to run a configured module step. Its progress, result, logs, outputs, module version, permissions, and responsible identities are recorded independently from the reusable step definition. Successful completion advances the Task circle through the same visible route as completion of human work.

## User Account and Actor Record

A **User Account** is an authenticated identity that can access the product.

An **Actor Record** represents a person or automated actor inside work. Not every actor record requires a user account. A contractor, candidate, external authority, or historical participant may be represented without being invited to log in.

A user account may be linked to one person actor record. Authentication identity, work participation, and knowledge evidence must remain conceptually distinct.

## Creator, Initiator, Requester, Executor, Participant, and Approver

These roles are deliberately distinct.

### Creator

The **Creator** is the actor that technically created a record.

### Initiator

The **Initiator** started a particular execution. An automated integration may create a record on behalf of a human initiator.

### Requester

The **Requester** owns the need for the result and is responsible for accepting or rejecting it unless authority is explicitly delegated.

For a native work node, the creator becomes the requester by default.

### Executor

The **Executor** is the actor currently responsible for performing an actionable step.

### Participant

A **Participant** is any actor involved in the lifecycle or workflow of a work node.

### Approver

An **Approver** is authorized to approve a particular step or result. The final approver may be the requester or an explicitly delegated actor.

These roles may belong to the same person, but the model must not assume that they always do.

## Role

A **Role** describes a function required in the context of work rather than a named person.

Examples include architect, requester, structural engineer, database administrator, reviewer, or permit authority.

A role can be used in templates before a concrete participant is known.

## Capability

A **Capability** describes an ability, qualification, permission, or area of expertise required to perform work.

Examples include:

- design residential foundations;
- administer ClickHouse;
- program in Python;
- approve expenses up to a specified amount;
- perform a legally valid electrical inspection.

A capability requirement may include a level, evidence, validity period, location, or certification.

Roles and capabilities are related but different. “Database engineer” is a role; “operate ClickHouse clusters” is a capability.

## Capability and Knowledge Claim

A **Claim** states that an actor has a capability or knowledge of a subject at a particular level.

Claims from different sources must remain distinguishable:

- self-declared by the person;
- confirmed by an administrator or authorized reviewer;
- supported by explicit evidence;
- inferred from accepted work history;
- imported from an external system;
- disputed, stale, or revoked.

An administrator's confirmation must not silently overwrite a person's self-assessment. Multiple claims can coexist, and the UI should show their sources, evidence, confidence, and review dates.

## Knowledge Subject

A **Knowledge Subject** is a concrete entity about which an actor may hold contextual knowledge. Initial types include service, project, system, domain, and custom/other.

Knowledge of a subject is distinct from a general capability. Two people may both know Python while only one knows the architecture, history, and operating constraints of Product A. The number of independent knowledge holders is therefore a continuity signal: zero holders means the subject is uncovered, one holder is a concentration risk, and multiple holders indicate that knowledge is distributed. This diagnostic describes risk around the subject and must not be presented as a negative label on the person.

A work node may require both capabilities and knowledge subjects. Matching uses conjunction: an actor is eligible only when they satisfy every required role, skill, and subject-specific knowledge condition.

## Service

A **Service** is a maintained capability or system that delivers ongoing value and has explicit ownership.

Examples include an analytics platform, payment service, home electrical system, or external legal service.

Services may be associated with work nodes, capabilities, actors, incidents, and operational history. A service is not necessarily a containment parent.

## Workflow

A **Workflow** describes how a work node progresses through human and automated steps.

It may contain sequential, conditional, parallel, reusable, and failure-handling paths.

A workflow definition is distinct from a workflow execution. Editing a definition must not silently rewrite an active execution.

The MVP executes human steps sequentially. Each step is matched independently from its role, skill, and entity-knowledge requirements. Completing a step makes the next one ready; after the final step, control returns to the requester for acceptance. API, script, and module steps share the conceptual sequence but require dedicated execution safeguards before they are enabled.

An actor's personal work queue is a projection of ready and active workflow steps across the forest. It is not a separate task list or execution surface: every entry navigates to its node in the tree, where the root purpose, surrounding branch, requester, requirements, history, and available actions remain visible.

The same projection appears as numbered markers on the current tree. These markers answer “what should I work on first?” for the viewing actor. They are derived from active work, readiness, temporary criticality, explicit cross-root decisions, and matching context, and therefore are never stored as an isolated priority number on a node.

Final acceptance is also personal work. A requester can accept the result or return a specific completed step with a reason. Returning a step makes it ready, resets all later steps to waiting, and records both the decision and explanation without discarding the earlier execution history.

## Workflow Step

A **Workflow Step** is one unit within a workflow.

A step defines:

- the expected outcome;
- the required role or capability;
- the actor selection policy;
- input and output data;
- completion conditions;
- timeout and failure behavior;
- permitted transitions.

A step can be performed by a human or automated actor.

## Template

A **Template** is a versioned, reusable definition derived from designed or completed work.

A template may include:

- work-node structure;
- relative dependencies;
- workflows;
- roles and capability requirements;
- expected artifacts;
- resource categories and estimates;
- relative dates and durations;
- acceptance criteria;
- decision points;
- configurable parameters.

A template should not expose private execution data by default. Named people, exact addresses, account identifiers, confidential documents, and other instance-specific values must be removed, generalized, or converted into parameters before sharing.

## Execution

An **Execution** is a concrete use of a template or workflow.

It contains actual participants, dates, costs, resources, events, decisions, artifacts, and results. An execution retains the version of the template from which it was created.

Changes discovered during execution may be proposed as improvements to a future template version, but they do not automatically mutate the template.

## External Work Candidate

An **External Work Candidate** is a provider-neutral proposal to create or update native work, normalized by the separate Integration Gateway from an object such as a Jira or GitHub issue.

It carries the content and evidence needed to recommend placement, but it is not a Work Node and is not a provider-shaped entity in the Work Graph core. Provider identity, raw fields, synchronization state, and field-ownership rules remain in the gateway.

When placement is confirmed, the gateway creates an ordinary native Work Node through the public domain API and stores the external-to-native identity mapping itself. A candidate without a sufficiently confident parent remains in the gateway's **Unplaced** inbox until a person resolves it.

## Priority Concepts

### Importance

**Importance** describes how much an outcome matters within a given context.

### Urgency

**Urgency** describes how time-sensitive action is.

### Readiness

**Readiness** describes whether the work can be acted on now.

### Cost of Delay

**Cost of Delay** describes the expected loss caused by postponement.

### Priority

**Priority** is an explainable recommendation about the relative order in which work should receive attention.

Priority may consider importance, urgency, contribution to parent outcomes, dependencies, critical path, risk, effort, readiness, resource constraints, and confidence in the available data.

### Queue Position

**Queue Position** is the ordering of actionable work for a particular decision context, actor, role, team, or service.

Priority and queue position are not permanent intrinsic properties of a work node. They may change as the context, time, dependencies, and resource availability change.

## Priority Context

A **Priority Context** defines the set of work and perspective within which priorities are compared.

Examples include:

- all root goals in a personal workspace;
- one house-building tree;
- a particular service;
- a team;
- a required capability;
- one person's currently actionable queue.

Cross-root comparison does not require adding a fake containment parent. The priority context supplies the comparison boundary explicitly.

## Priority Contention

**Priority Contention** occurs when two or more actionable work nodes compete for a constrained actor, capability, resource, budget, or time window and no existing preference evidence resolves the order.

The platform does not need a complete global ranking before contention exists. It can discover missing priority information lazily, at the moment when a decision becomes useful.

## Comparison Point

The **Comparison Point** is the meaningful place at which competing branches meet.

For two nodes in the same containment tree, this is normally their lowest common ancestor. For nodes in different root trees, it may be the workspace owner, a shared resource owner, or an explicit cross-root priority context.

The comparison point helps determine both what is being compared and who is authorized to answer.

## Comparison Authority

The **Comparison Authority** is the person or policy authorized to resolve contention between branches.

Authority may be derived from requester relationships, explicit delegation, workspace policy, resource ownership, or the comparison point. It must not be inferred merely from who happens to be assigned to a competing task.

## Priority Question

A **Priority Question** is a minimal question generated to resolve actual contention.

Examples include:

- Which should receive the shared capability first: Product A or Product B?
- Which deadline carries the greater cost of delay?
- If you cannot compare the products directly, which delegated product owner do you trust more for this decision?
- Is this temporary opportunity worth interrupting the current branch?

The user may answer, delegate, postpone, or state that they do not know. “Unknown” is a valid answer and may lead to a different, answerable question.

## Preference Evidence

**Preference Evidence** is a recorded answer or observation that helps resolve current or future contention.

It includes scope, author, rationale, confidence, time, and optional expiration. A preference between Product A and Product B should not automatically become a universal preference between every task proposed by their owners.

Preference evidence can become stale when goals, owners, trust, deadlines, or circumstances change.

## Trust and Delegation

**Delegation** grants another actor authority to make decisions within a defined scope.

**Trust Evidence** records a decision-maker's confidence in an actor for a particular role, capability, domain, or decision context.

Trust is contextual rather than a universal ranking of people. Preferring one product manager's judgment in product strategy does not imply preferring that person in security, finance, or engineering decisions.

Trust-based questions are a fallback when the responsible decision-maker cannot compare underlying branches directly. They must remain explainable and bounded in scope.

## Temporary Criticality

**Temporary Criticality** is a time-bounded condition that can elevate one node or branch because of an exceptional opportunity, threat, deadline, safety issue, or legal obligation.

Example: a tool required later in a house-building project is available today at a 90% discount. Purchasing it may become an immediate critical action even though the overall project is progressing slowly.

Temporary criticality must propagate visibly through the ancestor path to the root so that users understand why the branch demands attention. The propagation is an alert and explanation, not a silent permanent increase in the intrinsic importance of every ancestor.

When the tree becomes a template, the reusable knowledge is the triggering condition and response pattern—not the historical claim that the item is always critical.

## Tree Revision and History

A **Tree Revision** is a reconstructable state of the containment tree and its relevant relationships at a point in history.

**Tree History** records the events that produced revisions, including node creation, movement, replacement, deletion, restoration, relationship changes, recommendation decisions, and template-derived modifications.

History is part of the knowledge model. It supports temporal inspection, audit, comparison of alternative structures, recovery, and improvement of future template versions.

A node removed from the current tree may remain present in historical revisions and completed executions.

## Task Exchange

A **Task Exchange** is a discovery and matching surface where actors can inspect actionable human workflow steps, expected outcomes, required capabilities, constraints, and offered conditions, and can bid to perform them according to policy.

The offered unit is a human step rather than the whole containing task. A designer and programmer participating in one task submit independent duration estimates for their respective steps. The requester declares required capabilities and entity knowledge, but does not name a person or impose a minimum expertise level.

The task exchange is not part of the initial prioritization scope, but the domain model should preserve the distinction between:

- work being important;
- work being actionable;
- an actor being eligible;
- work being offered;
- an actor accepting or claiming it.

The initial selection policy chooses the shortest proposed duration. Once selected, it becomes the agreed estimate for the execution attempt. Later policies may adjust a proposal using the actor's observed estimation bias, stability, exceptional-overrun risk, evidence quality, workload, and knowledge-sharing effects. Zero coefficients with zero completed observations mean unknown reliability, not perfect reliability.

The complete agreed model and MVP sequence are recorded in [Human-Step Exchange](task-exchange.md).

## Recommendation

A **Recommendation** is a proposed change or action produced by the system.

Examples include:

- attach a work node to a suggested parent;
- change a priority or queue position;
- reuse an existing template;
- select a participant with matching capabilities;
- mark two work nodes as duplicates.

A recommendation should include evidence, confidence, scope, creation time, and the model or rule set that produced it.

## Decision on a Recommendation

A **Recommendation Decision** records whether a human accepted, rejected, or modified a recommendation and why.

These decisions provide an audit trail and can improve later recommendations. A rejected recommendation must not silently reappear without new evidence or an explicit reason.

## Provenance

**Provenance** explains where an entity or value came from.

Possible sources include:

- native user input;
- an external provider;
- a template version;
- an automated rule;
- an AI-assisted recommendation;
- a confirmed human decision.

Provenance is necessary for trust, synchronization, reuse, and explainability.

## Illustrative Example: Building a House

```text
Build a house                                  Root Work Node
├── Select and purchase land                   Work Node
│   ├── Verify zoning                          Child Work Node
│   └── Complete soil inspection               Child Work Node
├── Create the design                          Work Node
├── Obtain permits                             Work Node
├── Select contractors                         Work Node
├── Build the foundation                       Work Node
└── Accept the completed house                 Work Node
```

Related information remains connected without becoming misleading tree children:

```text
Select contractors
├── artifacts: quotations and contracts
├── decisions: selected contractor and rationale
├── participants: requester, architect, contractors
├── resources: budget and scheduled capacity
└── events: quotation received, contract signed
```

After completion, the tree can produce a versioned `Build a detached house` template. Concrete names, addresses, prices, dates, and confidential documents remain in the execution or become explicit template parameters.

## Important Distinctions

The following concepts must not be collapsed into one field or entity:

```text
Workspace != Root Work Node
Containment != Dependency
Work Node != Artifact != Resource
Creator != Initiator != Requester != Executor != Approver
Role != Capability != Person
Template != Execution
Workflow Definition != Workflow Execution
External Work Candidate != Work Node
Importance != Urgency != Priority != Queue Position
Recommendation != Human Decision
```

## Open Questions

- Which work-node types should be built in?
- Can one real-world outcome be represented in multiple workspaces without duplication?
- Which dependency semantics are required for the first release?
- How are shared templates licensed and attributed?
- Which instance data must always be removed before publishing a template?
- Can an artifact or resource later be promoted into a work node?
- How should conflicting priority contexts be displayed?
- How are permissions inherited through containment and relationships?
- Which concepts belong in the first domain model and which remain future extensions?
