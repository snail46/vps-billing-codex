-- name: CreateUser :one
INSERT INTO users (id, email, password_hash, status, locale, timezone)
VALUES ($1, $2, $3, 'active', $4, $5)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateUserLastLogin :exec
UPDATE users SET last_login_at = now(), updated_at = now() WHERE id = $1;

-- name: CreateAdmin :one
INSERT INTO admins (id, email, password_hash, status, display_name)
VALUES ($1, $2, $3, 'active', $4)
RETURNING *;

-- name: GetAdminByEmail :one
SELECT * FROM admins WHERE email = $1;

-- name: GetAdminByID :one
SELECT * FROM admins WHERE id = $1;

-- name: UpdateAdminLastLogin :exec
UPDATE admins SET last_login_at = now(), updated_at = now() WHERE id = $1;

-- name: AssignAdminRole :exec
INSERT INTO admin_roles (admin_id, role_id)
SELECT $1, id FROM roles WHERE key = $2
ON CONFLICT DO NOTHING;

-- name: ListAdminPermissions :many
SELECT DISTINCT permissions.key
FROM permissions
JOIN role_permissions ON role_permissions.permission_id = permissions.id
JOIN admin_roles ON admin_roles.role_id = role_permissions.role_id
WHERE admin_roles.admin_id = $1
ORDER BY permissions.key;

-- name: CreateUserSession :one
INSERT INTO user_sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetActiveUserSession :one
SELECT user_sessions.*, users.email, users.status AS user_status, users.locale, users.timezone
FROM user_sessions
JOIN users ON users.id = user_sessions.user_id
WHERE user_sessions.token_hash = $1
  AND user_sessions.revoked_at IS NULL
  AND user_sessions.expires_at > now();

-- name: TouchUserSession :exec
UPDATE user_sessions SET last_seen_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeUserSession :exec
UPDATE user_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: CreateAdminSession :one
INSERT INTO admin_sessions (id, admin_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetActiveAdminSession :one
SELECT admin_sessions.*, admins.email, admins.status AS admin_status, admins.display_name, admins.two_factor_enabled
FROM admin_sessions
JOIN admins ON admins.id = admin_sessions.admin_id
WHERE admin_sessions.token_hash = $1
  AND admin_sessions.revoked_at IS NULL
  AND admin_sessions.expires_at > now();

-- name: TouchAdminSession :exec
UPDATE admin_sessions SET last_seen_at = now() WHERE id = $1 AND revoked_at IS NULL;

-- name: RevokeAdminSession :exec
UPDATE admin_sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: CreateAuditEvent :exec
INSERT INTO audit_events (
  id, actor_type, actor_id, action, resource_type, resource_id,
  before_data, after_data, ip_address, user_agent, request_id, trace_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12);

-- name: UpsertAdminTOTPSecret :exec
INSERT INTO admin_totp_secrets (admin_id, ciphertext, nonce)
VALUES ($1, $2, $3)
ON CONFLICT (admin_id) DO UPDATE
SET ciphertext = EXCLUDED.ciphertext, nonce = EXCLUDED.nonce, created_at = now(), verified_at = NULL;

-- name: GetAdminTOTPSecret :one
SELECT * FROM admin_totp_secrets WHERE admin_id = $1;

-- name: EnableAdminTOTP :exec
WITH enabled AS (
  UPDATE admin_totp_secrets SET verified_at = now() WHERE admin_id = $1 RETURNING admin_id
)
UPDATE admins SET two_factor_enabled = true, updated_at = now()
WHERE id IN (SELECT admin_id FROM enabled);
