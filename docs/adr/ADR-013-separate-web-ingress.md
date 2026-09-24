# ADR-013: Separate user and administrator ingress ports

## Status

Accepted.

## Decision

The packaged Gateway exposes two listeners. Container port 80, published through `APP_PORT`, serves only the user portal and non-admin API. Container port 81, published through `ADMIN_PORT`, serves the administrator portal at `/` and only `/api/v1/admin/*`.

Both listeners reach the same backend service, while the backend continues to enforce separate user/admin sessions, CSRF secrets, and RBAC. External reverse proxies and tunnels must map the user and administrator domains to their respective host ports.

## Consequences

User and administrator domains can be routed independently without publishing the internal web containers. Requests for the admin control plane on the user listener, and user APIs on the administrator listener, return 404. Existing deployments must publish the new admin port and change `ADMIN_WEB_ORIGIN`; no database migration is required.
