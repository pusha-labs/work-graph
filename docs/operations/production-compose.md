# Production Compose Deployment

`compose.production.yaml` runs the native Work Graph MVP as an isolated three-service stack:

- PostgreSQL with persistent bind-mounted storage;
- the Work Graph API, reachable only inside the Compose network;
- the web application, bound to host loopback for a separate TLS reverse proxy.

Authentication endpoints keep shared rate-limit state in PostgreSQL. Repeated login attempts are limited by both account email and client address; public registration and invitation-password checks have separate limits. Reverse proxies must replace or correctly append `X-Forwarded-For` so Work Graph receives the real client address rather than a shared proxy address.

Password recovery is enabled when `WORK_GRAPH_PUBLIC_URL`, `WORK_GRAPH_EMAIL_FROM`, and `WORK_GRAPH_SMTP_ADDRESS` are configured. Set `WORK_GRAPH_SMTP_USERNAME` and `WORK_GRAPH_SMTP_PASSWORD` when the relay requires authentication. Reset links expire after 30 minutes, are single-use, and invalidate every existing session when the password changes. The request endpoint always returns the same response whether an account exists or email delivery is configured, so it does not disclose registered addresses.

Copy `.env.example` to `.env` and set a unique database password, a 32-byte base64 `WORK_GRAPH_MASTER_KEY`, the persistent data directory, and the loopback web port. Never commit the populated `.env` file.

Create the PostgreSQL data directory before starting because production Compose deliberately refuses to create a missing storage path silently. Then run:

```sh
docker compose -p work-graph -f compose.production.yaml up -d --build
```

The default listener is `127.0.0.1:8088`; expose it through an HTTPS reverse proxy rather than publishing the container directly to the network. The optional automation runner and Integration Gateway are separate deployment decisions and are not enabled by this baseline.
