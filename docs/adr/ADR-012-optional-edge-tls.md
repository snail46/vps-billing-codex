# ADR-012: Optional edge TLS for isolated deployments

## Status

Accepted.

## Decision

The application continues to serve HTTP inside its deployment network and does not terminate public TLS itself. Public ingress terminates TLS at an external reverse proxy or Cloudflare Tunnel and uses exact HTTPS origins with Secure cookies.

Production configuration remains HTTPS-strict by default. Operators may explicitly set `ALLOW_INSECURE_HTTP=true` for an isolated, trusted LAN deployment. In that mode exact HTTP or HTTPS origins are accepted and `COOKIE_SECURE=false` is allowed. The exception does not weaken secret validation, CSRF, RBAC, Audit, payment safety, or Provider transport requirements.

## Consequences

The prebuilt-image example starts on hosts without a public IP or certificate. Operators must set both Web origins to the exact browser-visible origin. LAN HTTP mode does not emit Secure cookies or HSTS and must not be exposed directly to the public Internet. Moving behind HTTPS requires `ALLOW_INSECURE_HTTP=false`, `COOKIE_SECURE=true`, and HTTPS origins; no database migration is required.
