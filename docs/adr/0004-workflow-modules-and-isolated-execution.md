# ADR 0004: Workflow Modules and Isolated Execution

**Status:** Accepted architecture direction  
**Date:** 2026-09-23

## Context

A Task circle must support both human work and automated work. A user should be able to add the next step through one universal entry point, type an intent such as `engineer`, `bash`, or `HTTP request`, select the interpretation, and then continue in an editor appropriate to that step type.

Built-in and third-party modules must be able to:

- declare when they match a user's query;
- define their configuration and validation;
- control the fields shown by the step editor;
- execute work asynchronously;
- report progress, completion, failure, and structured outputs;
- participate in the same visible route and immutable history as human steps.

Running third-party code in the API process, browser origin, or host Docker daemon would create an unacceptable security boundary.

## Decision

### Universal step composer

The **Add next step** interaction begins with one intent field. A registry searches human capabilities, entity knowledge, built-in step types, and installed module manifests. Selecting a result changes the editor to the selected type while preserving the original query.

Examples:

- `Python engineer who knows Product A` selects a human step and resolved matching requirements;
- `bash` selects a shell module and opens script, inputs, timeout, and output fields;
- `call API` selects an HTTP module and opens method, URL, headers, body, authentication, and output-mapping fields.

Search is deterministic in the first implementation. Natural-language interpretation may later be supplied by an optional resolver using embeddings or an LLM. Core operation must not require an LLM.

### Module contract

Each installed module exposes a versioned manifest containing at least:

- stable module ID, version, name, publisher, and compatibility range;
- search terms and supported step kind;
- a declarative configuration schema and UI hints;
- requested permissions, including network destinations, secrets, storage, and runtime needs;
- input and output schemas;
- execution image or other immutable runtime artifact identity;
- timeout, retry, cancellation, and idempotency capabilities.

Step instances store the module ID and version, validated configuration, granted permissions, and immutable execution references. Configuration is separated from secret values. Secrets are referenced by opaque IDs and are resolved only by the runner at execution time.

The initial editor uses declarative schemas. Third-party JavaScript is not loaded into the main application origin. If custom editors become necessary, they run in a sandboxed iframe on a separate origin with a narrow message protocol and no direct access to application credentials.

### Execution boundary

Automated steps are dispatched through an outbox to a separate runner service. The API never executes module code directly.

Each execution receives a short-lived, single-run identity and runs in a fresh sandbox with:

- a read-only runtime image and ephemeral writable storage;
- CPU, memory, process, output-size, and wall-clock limits;
- no host filesystem, database, metadata-service, or Docker-socket access;
- no network access by default, with explicit destination allowlists when granted;
- only the secret references explicitly approved for that step;
- authenticated, scoped APIs for progress and terminal results.

Containers are an initial packaging mechanism, not a complete security boundary. Internet-facing or multi-tenant deployments must add a hardened isolation layer such as rootless containers with seccomp/AppArmor and a sandboxed runtime, or microVMs. The native single-server edition may use the same protocol with a simpler runner but must preserve the process and permission boundary.

### Execution lifecycle

An execution is a durable record distinct from its workflow-step definition. Its lifecycle is:

`queued → running → succeeded | failed | cancelled | timed_out`

The runner leases queued executions. Progress and final reports include the execution ID, lease token, module version, timestamps, and structured outputs. Reports are authenticated and idempotent. A successful terminal report advances the Task circle; failure remains visible and follows the configured retry or human-intervention policy.

Logs and outputs are size-limited, access-controlled, and redacted for known secrets. Every installation, permission grant, configuration change, execution attempt, and result is attributable in history.

### Trust and distribution

Module installation is an administrative action. Publisher identity, artifact digest, requested permissions, and version changes are shown before approval. The architecture allows signed packages and trusted registries, but does not assume that all community modules are trustworthy.

## Consequences

- Human and automated steps share one route without forcing their editors or execution semantics to be identical.
- Bash and HTTP can be implemented as modules instead of permanent special cases in the domain model.
- Third parties can extend search, configuration, and execution through a stable contract.
- The UI can become module-driven before arbitrary third-party frontend code is allowed.
- Safe execution requires a runner service and operational hardening; it is intentionally not part of the current UI-only increment.
- Module versions and permissions become part of reproducible workflow history.

## MVP sequence

1. Introduce the universal step composer and registry-backed search.
2. Define the manifest, configuration schema, and stored step binding.
3. Implement outbound HTTP as the first constrained automated module.
4. Add the runner protocol and execution history.
5. Add Bash only after the isolation boundary and permission model are verified.
6. Publish a module SDK and compatibility tests for third-party authors.
