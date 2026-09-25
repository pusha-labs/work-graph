# Human-Step Exchange

## Purpose

The Work Graph exchange distributes executable human work without requiring the requester to name a person or declare a minimum expertise level.

The unit offered on the exchange is a **human workflow step**, not an entire task. A single task may contain several independently assigned steps:

```text
Requester -> Designer -> Programmer -> Requester acceptance
```

The designer and programmer see different offers, submit separate duration estimates, and accumulate separate execution evidence. Small collaborative work therefore remains one task and one circle; it does not need an artificial subtree merely to represent multiple professions.

Automated API, script, and module steps do not bid. Their timing and reliability may be measured separately.

## Eligibility and discovery

When a human step becomes ready for assignment, it is offered to every actor who satisfies all of that step's role, skill, and entity-knowledge requirements.

The requester describes **what kind of capability and knowledge the step needs**, but does not select a named employee and does not set a minimum self-reported expertise level. Claim levels and evidence sources help rank and assess candidates; they are not a requester-controlled exclusion threshold.

Eligibility, offering, bidding, selection, execution, and acceptance are distinct states. A person may be eligible without bidding, and winning a bid does not complete the step.

## Progressive disclosure and distribution policy

The exchange is a capability of the work model, not a mandatory interface for every workspace.

New workspaces use **Simple** distribution by default. In Simple mode, eligible people continue to start ready work directly. A person managing a private project, such as building a house without a choice of contractors, should not see an exchange tab or be asked to bid against themselves.

A workspace can explicitly enable **Exchange** distribution when competing candidates and duration estimates are useful. An unstarted human step can override the workspace policy with Simple or Exchange; otherwise it inherits the workspace setting. Automated steps never enter the exchange.

The interface follows the effective policy:

- hide exchange navigation when exchange distribution is not enabled and no exchange steps are visible;
- keep direct start/claim actions in Simple mode;
- reveal bids only for human steps whose effective policy is Exchange;
- preserve policy changes and existing bid history without forcing exchange concepts into the normal task route.

Later versions may recommend Exchange when several eligible performers repeatedly compete for the same work, but they must not silently change the policy.

## Bid

A bid belongs to one actor and one human workflow step. For the first implementation it contains:

- the proposed execution duration;
- the time the bid was submitted;
- its active, withdrawn, won, or lost state.

Later versions may also include earliest start time, confidence, conditions, capacity, or an explanation. Monetary pricing is outside the current product scope.

Each step has its own bids and its own winning actor. Estimates proposed for one step never implicitly apply to another step in the same task.

## Initial selection rule

The first deterministic policy selects the eligible active bid with the shortest proposed duration. Stable tie-breaking must be explicit and auditable; submission time followed by actor ID is sufficient for an initial implementation. After selection, the winning proposal becomes the agreed duration for that execution attempt.

This rule is deliberately simple. The selected bid, all competing bids, and the rule version must remain in history so that later selection models can be explained and evaluated.

The requester can inspect each candidate's observation count, average estimation bias, stability, and long-overrun history beside the bid. During this first evidence-visible phase those signals are contextual only: they do not silently alter the shortest-estimate result. This makes unknown reliability explicit while the future risk policy is still being designed and validated.

When every active candidate has at least five completed observations, the interface may also show an advisory risk outlook. The initial outlook estimates elapsed duration from the candidate's observed mean bias and adds a one-sided 95% uncertainty buffer (`1.645 × observed stability / √observations`). A floor prevents implausibly fast historical data from reducing expected duration below 25% of the proposal. The lowest outlook is visibly marked as an advisory, while the actual MVP selection policy remains the shortest raw estimate. If any candidate lacks sufficient evidence, no comparative risk recommendation is shown; a newcomer is therefore never treated as perfectly reliable or silently excluded.

## Execution observation

When the winning actor starts the step, the system records the execution start. When the actor completes it, the system records the finish and calculates actual execution duration for that actor's step attempt.

Agreed and actual duration belong to the workflow-step attempt, not to the containing task. Requester review time, time spent in earlier or later steps, and another participant's execution time must not be attributed to the winner.

The definition of paused or externally blocked time is deferred. Until pause semantics exist, the product must label the measured value honestly as elapsed execution time.

## Adaptive performer statistics

Every actor begins with zero-valued coefficients and zero observations. Zero with no observations means **unknown**, not perfectly accurate. Every display and selection policy must keep the observation count beside the coefficients.

After each completed winning step, the system updates at least these separate signals:

1. **Estimation bias** — the typical relative difference between agreed and actual duration. This captures systematic underestimation or overestimation.
2. **Estimation stability** — how consistently the actor stays near that typical difference.
3. **Long-overrun risk** — the frequency and severity of exceptional cases where actual duration is much longer than agreed.
4. **Observation count** — how much evidence supports the statistics.

The coefficients belong to the actor's execution history and are updated from step attempts. Future versions may additionally scope them by capability, entity, work type, or time window when enough observations exist.

Updates must be incremental and reproducible from immutable attempt observations. A single aggregate score must not erase the underlying agreed duration, actual duration, or exceptional overruns.

## Future risk-aware selection

The shortest proposed estimate is only the initial policy. Once enough evidence exists, selection may consider:

- risk-adjusted expected completion time;
- estimation bias and stability;
- long-overrun risk;
- relevant skills and entity knowledge;
- evidence source and confidence;
- current load and earliest availability;
- knowledge concentration and knowledge-sharing benefit;
- the value of safely involving a new person;
- branch priority and temporary criticality.

The winner may therefore differ from the person who entered the smallest raw duration. The system must explain the selected policy, material evidence, risk adjustment, and trade-offs. Unknown newcomers must not be treated as perfectly reliable merely because their coefficients are zero, and they must not be permanently excluded for lacking history.

## Native MVP sequence

The first exchange slice should be implemented in this order:

1. Persist bids against an unclaimed, ready human workflow step.
2. Show eligible ready steps in a personal exchange view.
3. Let an eligible actor submit or replace a proposed duration and withdraw before selection.
4. Show the requester the bids for each step.
5. Select the shortest active estimate deterministically and record the policy decision.
6. Assign only that workflow step to the winner.
7. Record the selected estimate as the agreed duration in the resulting execution attempt.
8. Record elapsed execution duration at completion.
9. Incrementally update actor statistics while keeping zero observations distinguishable from reliable history.

The first slice does not require monetary offers, minimum expertise levels, a mature risk model, parallel workflow branches, or automatic knowledge-sharing optimization. Its data model must preserve the evidence needed to add those policies later.
