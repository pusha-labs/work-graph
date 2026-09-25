# ADR 0001: Application Stack

**Status:** Accepted  
**Date:** 2026-09-22

## Context

Work Graph must be efficient for self-hosting, approachable to contributors, and capable of horizontal scaling. The local development machine does not have a current Node.js or Go runtime.

## Decision

Use:

- Go 1.27 for the backend;
- React 19 with TypeScript for the web interface;
- Node.js 24 LTS for frontend tooling;
- PostgreSQL 18 for primary persistence;
- Docker Compose as the reproducible development and initial deployment environment.

The Go backend starts as a modular monolith. Modules communicate through explicit application services, commands, and domain events rather than modifying one another's state directly.

## Consequences

- The product can run as a small number of containers.
- Backend modules can later become separate workers or services.
- Host runtime versions are not required for normal containerized development.
- PostgreSQL-specific optimizations must remain behind persistence boundaries where practical.
