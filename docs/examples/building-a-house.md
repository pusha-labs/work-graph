# Example: Building a House

**Status:** Exploratory example  
**Purpose:** Demonstrate how Work Graph can represent a large personal project without requiring a company, Jira, or a predefined project hierarchy.

## Scenario

A person wants to build a house.

The desired result is large, expensive, long-running, and dependent on many kinds of work. It involves personal decisions, professionals, authorities, suppliers, purchases, documents, inspections, and automated reminders.

A conventional task list can record individual actions, but it does not necessarily preserve why they were performed, how they depended on one another, who was required, what was purchased, and which parts of the experience should be reused.

In Work Graph, `Build a house` becomes a root work node.

## Initial Tree

```text
Build a house
├── Define requirements and budget
│   ├── Describe household requirements
│   ├── Define location constraints
│   ├── Establish the initial budget
│   └── Define acceptance criteria
├── Select and purchase land
│   ├── Search for suitable land
│   ├── Verify zoning restrictions
│   ├── Complete soil inspection
│   ├── Review legal ownership
│   └── Complete purchase
├── Design the house
│   ├── Select an architect
│   ├── Produce the initial design
│   ├── Estimate construction cost
│   ├── Revise the design
│   └── Accept the final design
├── Obtain permits and approvals
│   ├── Prepare the application
│   ├── Submit the application
│   ├── Respond to authority questions
│   └── Receive permission to build
├── Prepare construction
│   ├── Select contractors
│   ├── Plan procurement
│   ├── Arrange temporary utilities
│   ├── Prepare the site
│   └── Approve the construction schedule
├── Construct the house
│   ├── Build the foundation
│   ├── Construct the structure
│   ├── Install the roof and exterior
│   ├── Install utilities
│   ├── Complete the interior
│   └── Complete external works
├── Inspect and correct
│   ├── Perform required inspections
│   ├── Record defects
│   ├── Correct defects
│   └── Confirm compliance
└── Accept and occupy the house
    ├── Complete final walkthrough
    ├── Collect warranties and manuals
    ├── Accept the completed result
    └── Move in
```

This is illustrative rather than universal. A real tree depends on location, building type, financing model, regulations, and the owner's choices.

## Containment and Dependencies

The tree explains purpose, not complete execution order.

For example, `Build the foundation` belongs under `Construct the house`, but it may depend on work in other branches:

```text
Receive permission to build ───────┐
Complete soil inspection ──────────┼──▶ Build the foundation
Accept the final design ───────────┤
Select foundation contractor ──────┘
```

These are dependency edges. They do not give `Build the foundation` multiple containment parents.

## Participants and Capabilities

The reusable process should not require particular names. It should describe required roles and capabilities.

```text
Select and purchase land
├── requester: future homeowner
├── required role: real-estate legal adviser
├── required capability: verify land ownership
├── required capability: interpret local zoning rules
└── possible authority: local municipality

Design the house
├── requester: future homeowner
├── required role: architect
├── required capability: residential building design
├── required capability: local building-code compliance
└── final acceptance: future homeowner
```

During a concrete execution, the system resolves roles to actual people, companies, or public bodies. The execution records who participated and why they were selected.

## Human and Automated Workflow

The node `Select a contractor` can contain an executable flow:

```text
Requester defines requirements
        ↓
System sends requests for quotation
        ↓
Contractors submit quotations
        ↓
System checks required documents
        ↓
Architect performs technical review
        ↓
Requester compares qualified offers
        ↓
Requester records the decision
        ↓
System sends acceptance and rejection notices
        ↓
Contract is prepared and signed
```

The participants in this flow include people and automated actors. Every transition should remain visible and auditable.

## Work and Related Information

Not every related object becomes a child in the work tree.

For `Purchase windows`, the system may record:

```text
Purchase windows                              Work Node
├── quotations                                Artifacts
├── technical drawings                        Artifacts
├── signed order                              Artifact
├── windows and installation materials        Resources
├── allocated budget                          Resource
├── selected supplier and rationale           Decision
├── order placed                              Event
├── delivery completed                        Event
└── supplier and installer                    Participants
```

This preserves useful experience without mixing documents, physical products, people, and activities into one ambiguous hierarchy.

## Explainable Priority

The importance of the root goal propagates context to its descendants, but it does not make every house-related task equally urgent.

Assume construction should begin in ten weeks. The system may recommend `Submit permit application` as the next highest-priority task because:

- the permit is required before construction;
- the authority's expected response time is eight weeks;
- the application is ready;
- several later tasks depend on approval;
- delaying submission threatens the planned construction start.

At the same time, `Choose interior paint colors` may be important but not yet urgent or blocking.

An explanation can be shown as:

```text
Recommended position: highest actionable priority

Reasons:
- supports the active root goal "Build a house";
- blocks 12 downstream work nodes;
- expected external lead time is 8 weeks;
- planned construction start is in 10 weeks;
- required application artifacts are ready;
- requester approval is available.
```

The user may override the recommendation, but the override records its author and rationale.

## Acceptance and Closure

Completing the final construction activity does not automatically close the root goal.

The result returns to the requester for acceptance. Acceptance may require:

- a final walkthrough;
- resolution of recorded defects;
- delivery of warranties and manuals;
- confirmation of permits and inspections;
- comparison with the original desired outcome;
- explicit approval by the homeowner.

Execution completion and requester acceptance remain separate events.

## Learning from the Execution

During the project, the execution accumulates evidence:

- actual durations;
- actual costs;
- suppliers considered and selected;
- delays and their causes;
- rejected design alternatives;
- required documents;
- inspection results;
- missing or unnecessary steps;
- decisions and their consequences.

This information remains attached to the concrete execution.

## Creating a Reusable Template

After the house has been accepted, the owner can propose creating a template from the completed tree.

```text
Template: Build a detached house
├── Define requirements and constraints
├── Select and verify land
├── Design the house
├── Obtain local approvals
├── Select qualified contractors
├── Plan procurement
├── Construct the house
├── Inspect and correct
└── Accept and occupy
```

Concrete execution data must be transformed before reuse:

| Concrete execution value | Reusable template value |
| --- | --- |
| Homeowner | Requester / future homeowner |
| A named architect | Licensed residential architect |
| A specific municipality | Local permit authority |
| A specific street address | Project location parameter |
| Exact calendar dates | Relative dates and lead times |
| Actual contract amount | Budget parameter or estimate range |
| Named supplier | Supplier selection requirement |
| Private contract | Expected contract artifact |

The template retains structure, required roles, capability requirements, dependencies, expected artifacts, decision points, and lessons learned. It does not publish personal data or confidential documents by default.

## Reusing the Template

Another user can create a new execution from the template and supply parameters:

```text
Location: Utrecht, Netherlands
House type: detached residential house
Target completion: September 2030
Budget range: user-provided
Requester: current user
```

The new execution receives its own participants, dates, decisions, costs, artifacts, and events. It remains connected to the template version that created it.

The user can modify the generated tree without changing the shared template. Improvements discovered during the execution may later be proposed for a new template version.

## What This Example Demonstrates

The house-building scenario establishes several product requirements:

1. A root does not require an organization above it.
2. A workspace can contain multiple unrelated root goals.
3. The containment tree and dependency graph must be separate.
4. Roles and capabilities must be reusable without named participants.
5. Work, artifacts, resources, decisions, and events are distinct concepts.
6. Priority must be derived from context and execution readiness.
7. Human and automated actors can participate in the same workflow.
8. Completion and requester acceptance are separate.
9. A concrete execution and a reusable template are different entities.
10. Template publication requires privacy-aware generalization.

## Open Questions Raised by the Example

- How much domain-specific knowledge should the core product contain?
- Who is allowed to publish or share a template derived from collaborative work?
- How should regional variations in laws and permits be represented?
- How should estimated and actual costs be modeled?
- Can several users combine improvements into a shared template version?
- How should the system warn users that a template is guidance rather than professional legal, engineering, or safety advice?
