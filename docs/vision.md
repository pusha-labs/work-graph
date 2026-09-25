# Project Vision

**Status:** Initial draft  
**Version:** 0.1  
**Purpose:** Describe why the project exists, what makes it different, and which principles should guide product and architecture decisions.

## Summary

This project is an open-source platform for understanding, prioritizing, executing, and reusing work.

It connects goals, projects, tasks, people, capabilities, and automated actions into a coherent work graph. Its central structural element is a tree of ownership and purpose: every native work item belongs to something larger, and its importance can be traced back to the goals it supports.

The platform can operate independently or alongside existing work-management systems such as Jira. In companion mode, it does not require an immediate migration or attempt to replace established workflows. Instead, it adds a missing intelligence layer: goal alignment, explainable prioritization, reusable operational knowledge, capability-based assignment, and transparent orchestration of human and automated work.

The platform is designed for individuals, families, teams, communities, and organizations. It does not require a company, department, portfolio, or fixed organizational hierarchy. Any meaningful outcome—from building a house to operating a data center—can become the root of a work tree.

The long-term goal is to help people and organizations answer four fundamental questions:

1. Why does this work exist?
2. What is the most valuable work that can be done next?
3. Who or what is best suited to perform it?
4. How can successful work be reproduced and improved?

## The Problem

Most task-management systems are effective at recording operational facts:

- what needs to be done;
- who is currently assigned;
- what status a task is in;
- when it is due;
- what comments and changes were made.

They are much less effective at explaining the structure and purpose of work:

- why a task exists;
- which goal it contributes to;
- how much it matters relative to other work;
- what will happen after it is completed;
- which work it blocks or enables;
- what roles, capabilities, services, and automated systems are required;
- whether similar work has already been completed;
- whether a proven execution pattern can be reused;
- who is responsible for accepting the final result.

As a result, people and organizations accumulate large collections of tasks, tickets, projects, comments, and status changes without building a reliable model of how their work creates value.

History is preserved, but experience is not made reproducible.

### Priority without context

In conventional task trackers, priority is usually a manually selected property of an individual ticket: `Highest`, `High`, `Medium`, or `Low`.

This field rarely explains:

- why the task is important;
- which objective gives it importance;
- what the cost of delay is;
- how many other tasks depend on it;
- whether it is ready to be performed;
- whether the required capability is available;
- when and why the priority was changed.

Different people interpret the same priority scale differently. Over time, many unrelated tasks become “high priority,” while the system remains unable to determine which available action creates the most value now.

The project treats this as a structural problem rather than a missing field. Meaningful prioritization requires a model of goals, containment, dependencies, time, risk, capabilities, and execution readiness.

## Vision

We want work to be represented not as an isolated collection of tickets, but as an executable graph of practical knowledge.

Every work item should have an understandable place within a larger purpose. Every recommended priority should be traceable to that purpose and to observable operational conditions. Every workflow should make the movement of work between people and automated actors visible. Every successfully completed branch should be capable of becoming a reusable foundation for future work.

In the long term, the platform should help an individual, group, or organization:

- connect strategic goals to daily execution;
- identify the most valuable work that is currently actionable;
- distinguish importance from urgency and readiness;
- identify orphaned, duplicated, obsolete, or contradictory work;
- select participants based on required capabilities rather than hard-coded names;
- coordinate people, APIs, scripts, services, and AI agents in one transparent flow;
- convert completed work into versioned, reusable operating knowledge;
- improve processes using evidence from real executions;
- retain human control over consequential structural decisions.

## Core Principles

### 1. The model does not require an organization

A workspace may contain multiple independent root goals. Each root begins a tree of purpose and execution.

For example, a single person may maintain separate trees for building a house, changing careers, and learning a language. A company may maintain trees for launching a product, operating a service, and opening a data center. The underlying model is the same.

A technical workspace root may exist for storage, access control, and synchronization, but it is not part of the user's work model and must not influence priorities.

The future Jira companion experience is a narrow integration mode, not a limitation of the domain model.

### 2. Work does not exist without context

Every non-root native work item must belong to a parent. A root work item represents a meaningful desired outcome and does not require an organizational container above it.

If the correct parent is not yet known, the item must appear in a visible unclassified-work area. It must not silently disappear into a flat backlog.

The platform distinguishes two structures:

- **The containment tree** explains what larger outcome a work item belongs to. A work item has one primary parent in this tree.
- **The dependency graph** represents blocking, enabling, informational, and other cross-branch relationships. A work item may have many such relationships.

This separation preserves a clear chain of purpose without hiding the real complexity of execution.

### 3. Priority is a consequence of context

Priority is not merely a label attached to a ticket. It is a contextual and explainable recommendation derived from the work graph.

The platform should be able to consider:

- the importance of ancestor goals;
- the work item's expected contribution to its parent outcome;
- urgency and deadlines;
- cost of delay;
- blocking and enabling relationships;
- position on a critical path;
- operational and strategic risk;
- execution readiness;
- required effort;
- availability of the necessary capabilities;
- confidence and completeness of the underlying information.

Importance should propagate through the tree, but not blindly. Two tasks under the same important goal may have very different operational priorities. A task that unlocks fourteen other tasks may need to be performed before a larger task that cannot yet begin.

Priority is also relative to a decision context. The same work item may have:

- a workspace-wide priority across multiple root goals;
- a priority within a goal or project;
- a priority for a service;
- a priority for a role or team;
- a position in an individual's queue of actionable work.

The platform should explain recommendations rather than only display a score. A useful explanation might state that a task supports a critical objective, blocks several ready tasks, has a near deadline, and can be performed by an available engineer.

Manual overrides are allowed, but they should retain the author, rationale, timestamp, and—when appropriate—an expiration condition. An emergency override should not silently become a permanent truth.

Priority information does not need to be fully specified in advance. When two actionable branches compete for the same constrained capability, the platform should locate their meaningful point of intersection, identify who has authority to compare them, and ask the smallest useful question. The answer becomes reusable preference evidence rather than an unexplained permanent score.

If the responsible person cannot compare the work directly, the system may ask a question they can answer, such as which delegated owner they trust more in that context. Uncertainty is a valid state; the platform should not manufacture precision where the decision-maker lacks information.

### 4. Tree history is part of the knowledge

The current tree alone is not a complete representation of experience.

The platform must preserve how the tree changed over time: which nodes and branches were created, moved, replaced, removed, or restored; which dependencies changed; which recommendations were accepted; and why decisions were made.

Users should be able to inspect earlier revisions and understand the evolution of the work. History should support learning: ineffective steps can be identified, a better structure can be proposed, and successful changes can become part of a new template version.

Deletion from the current tree must not silently erase historical knowledge or break the integrity of past executions.

### 5. Completed work should become reproducible

A completed project should not remain only an archive of events.

Users should be able to turn a successful branch of work into a reusable template that preserves:

- task structure;
- ordering and dependencies;
- required roles and capabilities;
- automated actions;
- transition conditions;
- acceptance criteria;
- useful instructions and artifacts.

A template and an execution created from that template are different entities. Changes made during one execution must not silently modify the template, and template updates must not break running executions.

Templates should be versioned so that their owners can understand how practical knowledge evolves.

### 6. A task can be an executable process

A task may contain a visible execution flow made of multiple steps.

A step may represent:

- work performed by a person;
- work offered to a qualified group;
- review or approval;
- an API request or webhook;
- a script;
- an external service or software module;
- a decision or condition;
- an AI agent;
- a parallel or reusable subprocess.

The platform should make it clear:

- which step is active;
- who or what is responsible;
- why that participant was selected;
- what input was provided;
- what result is expected;
- what happens next;
- why execution stopped, failed, or changed direction.

Automation must not become an invisible collection of rules that users cannot understand or audit.

### 7. The requester remains accountable for acceptance

For a native task, its creator becomes the requester and first participant by default.

After the execution flow is complete, the result returns to the requester for acceptance. Execution and acceptance are distinct actions. A task is not necessarily complete merely because the last implementation step finished.

The domain model should distinguish:

- record creator;
- process initiator;
- requester or customer;
- current executor;
- participant;
- result approver.

These roles may initially belong to one person, but they must not be treated as permanently identical.

### 8. Processes request capabilities, not names

Reusable work should not depend on a particular employee.

Instead of encoding “assign this step to Ivan,” a process should be able to request:

- a role;
- one or more capabilities;
- capability levels;
- knowledge of a particular service;
- required access or certification;
- availability and capacity;
- location or time-zone constraints;
- other policy requirements.

The concrete participant can then be selected when the work becomes actionable.

The platform should explain why a person or group was suggested. Depending on organizational policy, the final assignment decision may remain with a human.

### 9. Humans retain control

Recommendations must not silently rewrite the structure or responsibility of work.

When the platform recommends a new parent, priority, template, participant, merge, or workflow change, it should show the evidence and confidence behind that recommendation.

Consequential structural changes require human confirmation unless the workspace owner has explicitly authorized a corresponding automation policy.

### 10. Integration should not require migration

The platform must be useful without forcing users to abandon their current task-management system.

In companion mode, an external system such as Jira may remain the system of record for:

- title and description;
- external status;
- comments and attachments;
- the assignee represented in the external system;
- provider-specific fields.

Our platform may own additional information such as:

- placement in the global containment tree;
- relationship to goals;
- capability requirements;
- explainable priority recommendations;
- execution flows;
- template provenance;
- structural decision history.

Every synchronized field must have a clearly defined owner. The platform should not create two independently editable copies of the same task without explicit conflict and synchronization rules.

### 11. Integrations are adapters, not the domain model

The core model must not depend on provider-specific concepts such as Jira Epic, GitHub Issue, or ServiceNow Case.

External systems should connect through adapters in a separate Integration Gateway. The gateway translates their objects and events into provider-neutral work candidates, owns identity mappings and synchronization state, and uses stable core APIs. This should eventually allow the same core to work with:

- Jira;
- GitHub Issues;
- GitLab;
- YouTrack;
- Linear;
- ServiceNow;
- other work systems;
- native work items created directly in the platform.

### 12. Security and transparency are product requirements

The platform may process highly confidential personal and organizational data. Security cannot be postponed until an enterprise edition.

Users must be able to understand:

- which data is imported;
- where it is stored;
- which permissions an integration has;
- which data is sent to external services;
- whether an external AI model is involved;
- what automated actions were performed;
- how to disconnect a source and remove imported data.

Self-hosted installations must be able to operate without sending work data to external AI providers.

## Product Modes

### Standalone mode

The platform acts as the primary system for structuring and understanding work, including:

- root goals and child work nodes;
- containment and dependency relationships;
- people and automated actors;
- capabilities and subject-specific knowledge;
- explainable priorities;
- tree history and revisions;
- reusable templates and executions.

The first product increment will implement a deliberately small subset of standalone mode. It must be useful from an empty workspace before any external integration exists.

### Companion mode

The platform connects to an existing work-management system and augments it.

The initial companion experience, developed after the native product is usable, should be read-only:

1. Import work items and existing relationships.
2. Build a preliminary containment tree and dependency graph.
3. Identify work with no meaningful parent.
4. Suggest possible parents and explain each suggestion.
5. Find related or similar completed work.
6. Preserve human confirmations and corrections.
7. Use those decisions to improve future recommendations.

As trust grows, users may explicitly allow confirmed changes or automated actions to be written back to the external system.

Companion and standalone modes should share the same domain core.

The companion is not a synchronous proxy in front of Jira. A separate Integration Gateway owns provider credentials, raw data, mappings, retries, conflicts, and unresolved placement. Work Graph owns native nodes and asks humans for help through an Unplaced inbox when a candidate cannot be placed confidently. The accepted boundary is detailed in [ADR 0005](adr/0005-integration-gateway-boundary.md).

## Initial Target User

The first target user is a person or small group managing a real, multi-step project whose structure, participants, dependencies, and history no longer fit comfortably in a flat task list.

The reference project is building a house. It is complex enough to exercise goals, dependencies, specialists, purchases, decisions, documents, and changing plans without requiring corporate infrastructure.

## First User Journey

1. The user deploys the platform locally or opens a hosted instance.
2. The user creates a workspace and a root goal named `Build a house`.
3. The user creates, edits, and removes work nodes in a containment tree.
4. The user adds dependencies between branches.
5. The user records people, capabilities, and knowledge of relevant products, services, systems, or other subjects.
6. The user associates required capabilities, participants, artifacts, resources, and decisions with work.
7. The platform preserves each structural change and can reconstruct earlier tree revisions.
8. The platform reports structural problems and knowledge-concentration risks.
9. The user continues managing the real project in the product without relying on Jira.

The first successful outcome occurs when a user can structure a real project, understand its current state, inspect how it evolved, and discover a risk that would have been difficult to see in a flat task list.

## Initial Product Scope

The first product increment should include:

- workspace and root-goal creation;
- native work-node creation, editing, and removal;
- containment-tree visualization and editing;
- dependency creation and graph inspection;
- reconstructable tree history and revision comparison;
- a directory of people and automated actors;
- user accounts with personal capability and knowledge profiles;
- capabilities and subject-specific knowledge records;
- a catalog of products, services, systems, projects, and custom knowledge subjects;
- explicit evidence and confidence for knowledge claims;
- a personal interface where users can inspect and maintain their own claims;
- an administrative interface for people, capabilities, subjects, and confirmations;
- basic structural diagnostics;
- basic knowledge-concentration diagnostics;
- documented database backup and restore procedures;
- Docker-based self-hosted deployment;
- a realistic house-building demonstration workspace.

The first product may display priority inputs and unresolved contention without implementing complete automated priority resolution. The data model and history must preserve the information required to add that behavior later.

## Explicit Non-Goals for the Initial Release

The initial release will not attempt to provide:

- a complete Jira replacement;
- a Jira connector or any other production external connector;
- a full Scrum or Kanban suite;
- a general-purpose BPM engine;
- real-time simultaneous editing;
- mandatory accounts for every person represented in a workspace;
- autonomous restructuring of work without confirmation;
- automatic employee selection and assignment;
- autonomous AI agents executing business processes;
- perfect organization-wide priority optimization;
- a marketplace of integrations;
- mobile applications;
- billing infrastructure;
- a complete enterprise authorization system;
- universal management of every kind of organizational data.
- a complete task exchange or labor marketplace.

These capabilities may be considered after the initial problem and product assumptions have been validated.

## Open-Source Strategy

The core product should be open and practically usable in a self-hosted environment.

A user should be able to use the open-source edition to:

- deploy the platform;
- connect a supported source;
- import and export their own data;
- build and edit the work tree;
- inspect dependencies;
- receive basic structural and priority recommendations;
- understand why recommendations were made;
- create custom adapters using documented interfaces.

Potential commercial offerings may include:

- managed cloud hosting;
- installation and operational support;
- support contracts and service-level agreements;
- enterprise identity and access management;
- advanced audit and compliance capabilities;
- high-availability deployment;
- specialized enterprise integrations;
- migration and process-modeling services;
- advanced administration and governance;
- secure organization-specific AI configurations.

The license and the boundary between the open core and commercial capabilities must be decided before accepting substantial external contributions.

## Measures of Early Success

The first phase is successful if:

- a user can deploy the product and create a useful workspace without an external task system;
- a real project such as building a house can be structured and maintained over multiple sessions;
- containment and dependencies remain visually and semantically distinct;
- earlier tree revisions can be reconstructed after nodes are edited or removed;
- members can inspect and maintain their own capability and subject-knowledge claims;
- administrators can manage people, capabilities, knowledge subjects, evidence, and confirmations;
- the platform detects a realistic structural or knowledge-concentration risk;
- users understand the evidence and provenance behind diagnostics;
- the installation can be backed up and restored using documented procedures;
- users want to continue managing real work in the product.

At this stage, repeated usefulness to a small number of real users matters more than registration counts, repository stars, or feature breadth.

## Long-Term Direction

After the initial scenario is validated, the platform may evolve toward:

- learning reusable structures from completed work;
- turning successful branches into versioned templates;
- comparing different executions of the same template;
- modeling roles, services, and capabilities;
- capability-based participant selection;
- visual human-and-machine workflows;
- API, script, webhook, and agent execution;
- requester acceptance and controlled closure;
- evidence-based process improvement;
- explainable organization-wide prioritization;
- native work management without an external provider.

The long-term ambition is to create an open operating system for work: a system that helps people and organizations understand what their work means, decide what matters now, execute it transparently, and reuse what they learn.

## Open Questions

The following decisions are intentionally unresolved:

- the final project name;
- the boundary of the open-source core;
- the narrowest initial customer profile;
- initial support for Jira Cloud, Jira Data Center, or both;
- the exact UI relationship with Jira;
- the first explainable priority algorithm;
- whether any AI capability belongs in the first release;
- rules for deleted, archived, and inaccessible external work;
- multi-organization and workspace boundaries;
- minimum self-hosting requirements;
- synchronization conflict behavior;
- the process for turning completed work into a reusable template.

These questions should be resolved through focused product documents and architecture decision records without weakening the principles defined here.
