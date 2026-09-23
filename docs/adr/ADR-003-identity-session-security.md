# ADR-003: Identity session security

## Status

Accepted.

## Decision

User and administrator identities use independent database session tables, cookie names, HMAC session secrets, and HMAC CSRF secrets. Only opaque random tokens leave the server; only keyed digests are persisted. Cookies are HttpOnly and SameSite Strict, and production uses the `__Host-` prefix with Secure enabled. Unsafe cookie-authenticated requests require an `X-CSRF-Token` obtained from the authenticated response.

Passwords use Argon2id with OWASP's minimum parameters (19 MiB memory, two iterations, one lane). Login and registration are fail-closed behind an atomic Redis rate limiter. Administrator authorization is server-side and every protected handler declares a permission. Audit writes are mandatory for high-risk administrator actions.

Administrator TOTP follows RFC 6238 with a 30-second period and a one-period clock window. Enrollment secrets are encrypted with AES-256-GCM under a dedicated deployment key and are never returned again after setup.

## Consequences

The browser never persists session or CSRF tokens in local storage. User credentials cannot authenticate administrator routes or vice versa. Redis unavailability temporarily blocks authentication attempts instead of weakening brute-force protection. Operators must provision four distinct secrets of at least 32 characters and enable secure cookies outside local development.
