# 14 — Security

普通用户、管理员、Agent/Provider credential 完全隔离。

优先 Secure HttpOnly Session Cookie；Cookie 修改请求启用 CSRF；生产 CORS allowlist。

密码强哈希；管理员 2FA；RBAC；Rate Limit。

Secrets 不进日志，不明文进普通配置；使用 credential_ref。

Provider/Agent 生产通信必须 TLS。

所有 ownership/RBAC 后端强校验，不能靠前端隐藏按钮。

Server 对所有响应设置 CSP、frame denial、MIME sniffing protection、referrer policy 与 permissions policy；Secure Cookie 模式同时发送 HSTS。生产默认拒绝非 HTTPS Web Origin 与非 Secure Cookie；只有显式设置 `ALLOW_INSECURE_HTTP=true` 才允许 exact HTTP Origin 和非 Secure Cookie，用于受信任内网且不得直接暴露公网。生产始终拒绝启用 Fake Payment、过短或复用的 Identity/Payment/Metrics secret。`/metrics` 使用独立 Bearer token 且不经公网 Reverse Proxy 暴露。

管理员可在 Admin Web 内注册 TOTP；setup/enable/login 均写 Audit。TOTP secret 经 AES-GCM 加密存储，Session、CSRF、TOTP、Payment 与 Metrics 密钥必须各不相同。
