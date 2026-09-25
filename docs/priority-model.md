# Priority Model

**Status:** Initial conceptual draft  
**Version:** 0.1  
**Purpose:** Define how Work Graph discovers, explains, and resolves priority without requiring every task to be scored in advance.

This document describes product behavior and domain semantics. It does not prescribe a final mathematical algorithm.

## Central Idea

Priority is not primarily a field that users must fill in when creating work.

Priority becomes necessary when available work competes for a constrained capability, person, resource, budget, or time window. At that moment, Work Graph should determine what is actually in conflict, locate the meaningful point where the competing branches meet, identify who can resolve the conflict, and ask the smallest answerable question.

The system should avoid forcing users to maintain a complete global ranking that may never be used.

## Motivating Example

A founder delegates two product goals:

```text
Founder
├── Product A
│   ├── requester / owner: Product Manager A
│   └── engineering task A
└── Product B
    ├── requester / owner: Product Manager B
    └── engineering task B
```

There are three programmers. One programmer becomes eligible for both engineering task A and engineering task B. Both tasks are actionable, but the programmer cannot perform both first.

The programmer should not be required to invent a business priority. The contention originates above the programmer, at the point where Product A and Product B compete for shared engineering capacity.

The system identifies the founder as the comparison authority and asks:

> Which should receive engineering capacity first: Product A or Product B?

The founder may not understand the two products well enough to answer. “I don't know” is valid information, not a failure.

The system can then ask a different question that the founder can answer:

> In this product decision, whose judgment do you trust more: Product Manager A or Product Manager B?

The answer creates scoped preference or trust evidence. It does not permanently mark every task from one manager as more important than every task from the other.

## Why Priority Can Be Lazy

Many branches never compete for the same constrained capacity. Ranking them in advance creates unnecessary work and false precision.

Work Graph should request additional priority information only when it can change a decision, such as:

- two actionable tasks require the same programmer;
- two purchases compete for one budget;
- two projects require the same specialist;
- a temporary opportunity interrupts planned work;
- multiple root goals compete for the user's available time.

This is **lazy preference elicitation**: learn just enough to resolve a real decision, then retain the answer with its context.

## Priority Is Not Status

A work node remains in its actual execution status regardless of priority.

Examples of status include:

- planned;
- blocked;
- ready;
- in progress;
- awaiting review;
- accepted;
- cancelled.

Priority does not move a task to another status. It helps choose among work that is relevant to the same decision context.

A blocked task may be extremely important but unavailable for immediate execution. It should influence the priority of work that can unblock it, while remaining outside the queue of actions that can be performed now.

## Priority Is Not a Single Global Score

The model distinguishes:

- **Importance:** how much an outcome matters in a context.
- **Urgency:** how time-sensitive action is.
- **Readiness:** whether action can begin now.
- **Cost of delay:** what is lost by waiting.
- **Dependency impact:** what other work is blocked or enabled.
- **Risk:** the expected consequence and likelihood of failure or delay.
- **Effort:** the resources required to create the result.
- **Confidence:** how reliable and complete the available information is.
- **Priority:** an explainable recommendation about relative attention.
- **Queue position:** an ordering for a particular actor or decision context.

These factors should remain inspectable. A single internal score may be useful for candidate ranking, but it must not erase the reasons or imply more certainty than the evidence supports.

## When Contention Exists

Priority contention requires all of the following:

1. Two or more work nodes are candidates for attention.
2. They compete for at least one constrained capability or resource.
3. Their order matters now or soon.
4. Existing policy and preference evidence do not already resolve the order.

If the nodes require different unconstrained actors, there may be no contention: both can proceed.

## Finding the Comparison Point

For competing nodes within one containment tree, the first candidate comparison point is their lowest common ancestor.

```text
Launch Product Portfolio                 Comparison point
├── Product A
│   └── Task A                           Candidate
└── Product B
    └── Task B                           Candidate
```

For nodes in different root trees, the comparison point can be supplied by:

- a workspace-level priority context;
- the owner of the shared resource;
- the person whose time is constrained;
- an explicit delegation or governance policy.

The comparison point defines the appropriate abstraction level for the question. The founder should normally compare Product A with Product B, not two low-level implementation tickets whose details they do not understand.

## Finding the Comparison Authority

The comparison authority can be determined from:

1. an explicit authority assigned to the comparison point;
2. the requester or owner of the common ancestor;
3. an explicit delegation for the affected resource or capability;
4. a workspace governance policy;
5. escalation to the next meaningful ancestor or context.

The current task assignee is not automatically the authority to choose business priority.

The resolution process should be recorded so users can understand why a particular person received the question.

## Asking the Smallest Useful Question

The first question should compare outcomes at a level the decision-maker is expected to understand.

Examples:

```text
Which outcome should receive the shared programmer first?

[Product A] [Product B] [Equal] [I don't know] [Delegate]
```

If the answer is unknown, the system can reduce or transform the question:

- Which deadline has a greater consequence if missed?
- Which product owner is authorized to decide for this shared capacity?
- Whose judgment do you trust more in this domain?
- Should the resource owner choose based on operational readiness?
- Should both branches receive a fixed share of capacity?

The system should not repeatedly ask semantically identical questions that have already been answered within a still-valid scope.

## Trust as Scoped Evidence

Trust can help when a decision-maker delegated domains they no longer understand in detail.

Trust must be scoped by:

- domain;
- role;
- decision type;
- affected branch or goal;
- time period;
- confidence.

For example:

```text
The founder prefers Product Manager A's judgment
for product prioritization
between Product A and Product B
for the current planning period.
```

This must not be interpreted as a universal statement that Product Manager A is more trustworthy in all matters.

## Reusing Answers

An answer becomes preference evidence with:

- the compared alternatives;
- the abstraction level;
- the decision context;
- the authority who answered;
- the rationale, when supplied;
- confidence;
- creation time;
- optional expiration;
- the circumstances that may invalidate it.

Future contention may reuse the evidence when the scope still matches. Otherwise, the system asks again.

Evidence may become stale after:

- a goal changes;
- a manager or requester changes;
- a deadline or budget changes materially;
- previous assumptions prove incorrect;
- the defined period expires;
- the decision-maker revokes or replaces it.

## Temporary Criticality

A child node should not silently become intrinsically more important than its parent branch. However, a specific node may become temporarily critical because of an exceptional condition.

Example:

```text
Build a house
└── Prepare construction
    └── Purchase tools
        └── Buy a required shovel today at a 90% discount
```

The purchase may deserve immediate attention because the opportunity expires today.

The critical condition must be visible along the full ancestor path:

```text
Build a house                         contains critical active branch
└── Prepare construction             contains critical active branch
    └── Purchase tools               contains critical active branch
        └── Buy shovel today          critical until offer expires
```

The system should record:

- the triggering condition;
- the expected benefit or avoided loss;
- the expiration time;
- who confirmed the interruption;
- which planned work was displaced;
- the actual outcome.

When this execution becomes a reusable template, the knowledge should be generalized:

```text
If a required long-lead or high-cost resource becomes available under a
time-limited favorable condition, evaluate early purchase against storage,
cash-flow, specification-change, and cancellation risks.
```

The reusable template must not mark `Buy shovel` as permanently critical.

## Propagation Through the Tree

The platform should propagate explanations, not only values.

Possible upward signals include:

- this branch contains an active critical condition;
- this branch blocks a high-importance outcome;
- this branch contains unresolved contention;
- this branch requires an authority decision;
- this branch is at risk because required capabilities are unavailable.

Possible downward context includes:

- ancestor importance;
- policy constraints;
- deadlines;
- accepted trade-offs;
- delegated authority;
- budget or capacity limits.

Propagation must preserve the source and meaning of each signal. An urgent descendant should not permanently rewrite every ancestor as urgent.

## Actionable Work and the Human-Step Exchange

The exchange presents human workflow steps that actors may be able to perform, including:

- expected outcome;
- required capabilities;
- constraints;
- status and readiness;
- offered conditions;
- requester and acceptance rules;
- relevant priority explanation.

The initial priority model should not attempt to design the full exchange. It must only preserve several distinctions:

```text
Important != Actionable
Actionable != Offered
Offered != Eligible
Eligible != Claimed
Claimed != Accepted as complete
```

Priority helps order relevant actionable opportunities when contention exists. It does not replace status, eligibility, or voluntary selection.

Offers and bids belong to individual human steps. Each eligible actor may propose a duration for that step. Once selected, that duration becomes the agreed estimate. The initial winner is the shortest estimate; later selection can use risk-adjusted expected time, estimation stability, long-overrun risk, workload, and knowledge-sharing policy. See [Human-Step Exchange](task-exchange.md).

## History Is Required

Priority decisions cannot be understood from the current tree alone.

The system should preserve:

- when competing branches appeared;
- which question was generated;
- who received it;
- whether they answered, delegated, or said they did not know;
- which evidence resolved the contention;
- which work proceeded;
- how the tree subsequently changed;
- whether the result confirmed or contradicted the decision.

Users should be able to move through tree history and inspect earlier revisions. This allows them to see which branches were created, removed, moved, or replaced and how those changes affected execution.

History makes it possible to identify inefficient steps and improve a template without rewriting what actually happened.

## Initial Explainability Contract

Every priority recommendation should be able to answer:

1. What alternatives were considered?
2. In which context were they compared?
3. What constrained resource created contention?
4. Which facts and prior decisions were used?
5. Who had authority to decide?
6. What is unknown or uncertain?
7. How long is the recommendation expected to remain valid?
8. What would cause it to be recalculated?

An example explanation:

```text
Recommendation: Offer engineering task A first.

Context:
- Task A and task B require the same available Python capability.
- Both tasks are ready.
- Their branches meet at the product portfolio owned by the founder.

Evidence:
- The founder could not compare the products directly.
- For the current planning period, the founder delegated this conflict to
  Product Manager A based on scoped product-strategy trust.

Confidence: medium
Valid until: end of the current planning period or a material goal change
```

## Initial Product Behavior

The first implementation should favor deterministic and inspectable behavior:

1. Determine which work is actionable.
2. Detect contention for an explicitly modeled capability or resource.
3. Attempt to resolve it using valid policy and preference evidence.
4. Find the comparison point.
5. Find or escalate to the comparison authority.
6. Generate a minimal comparison question.
7. Record the answer as scoped evidence.
8. Recalculate the affected queue and explanation.

AI may help summarize branches or formulate understandable questions, but authority resolution, evidence scope, expiration, and audit behavior must not depend on opaque model output.

## Current MVP Slice

The first implemented slice detects an eligible actor with actionable tasks under two or more root goals. The conflict is routed to a workspace owner or administrator. The decision maker first compares the goals; an “I don't know” answer can lead to a requester-trust comparison when the requesters differ. Decisions are append-only, the latest answer supplies an explainable pairwise preference, and Current work orders branches by pairwise wins. Expiration, automatic invalidation, deeper comparison points, and delegated authority remain future work.

The MVP also supports explicit temporary criticality for exceptional work. A signal includes a reason and deadline, propagates from its target node through every ancestor, expires automatically, and may be revoked by its creator or a workspace administrator. It is modeled separately from ordinary pairwise preference so a short-lived opportunity does not become permanent historical priority evidence.

Priority decisions and criticality changes are shown alongside structural revisions in the workspace activity timeline. Structural events can open the corresponding read-only tree revision; decision events preserve who acted, when, and the human-readable context without pretending to be tree snapshots.

## Non-Goals of the First Model

The first model will not attempt to:

- compute one objectively correct global score;
- ask users to rank every root or task in advance;
- infer universal trust rankings between people;
- automatically override human authority;
- design the complete task exchange;
- optimize all organizational capacity mathematically;
- treat historical answers as permanently valid;
- hide uncertainty behind numeric precision.

## Open Questions

- How is comparison authority represented when ownership is shared?
- When should a preference answer apply to descendant tasks automatically?
- How should ties and deliberately equal priorities affect resource allocation?
- What policies control repeated “I don't know” answers?
- When should the system delegate versus escalate a question?
- How should conflicting trust evidence be resolved?
- Which events invalidate earlier preference evidence?
- How should temporary criticality be approved in shared workspaces?
- How much history should be materialized as revisions versus reconstructed from events?
- How should the system measure whether a priority decision produced a better result?
