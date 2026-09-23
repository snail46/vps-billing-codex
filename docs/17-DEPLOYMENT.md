# 17 — Deployment

V1：reverse-proxy、user-web、admin-web、server、worker、postgres、redis。

环境：development/staging/production。

Health：
`/health/live`
`/health/ready`（检查 PostgreSQL/Redis，不因单个 Provider 离线让平台整体 unready）。

备份 PostgreSQL + 安全配置；必须定期恢复演练。

生产只允许版本 migration。

Identity 必需环境变量：`USER_SESSION_SECRET`、`ADMIN_SESSION_SECRET`、`USER_CSRF_SECRET`、`ADMIN_CSRF_SECRET`、`ADMIN_TOTP_ENCRYPTION_KEY`，每项至少 32 字符且必须各不相同；生产设置 `COOKIE_SECURE=true` 并配置精确的 `USER_WEB_ORIGIN` / `ADMIN_WEB_ORIGIN`。

首次管理员通过容器内命令创建，密码只从进程环境读取：

```sh
ADMIN_BOOTSTRAP_PASSWORD='replace-with-a-strong-secret' /app/bootstrap-admin --email admin@example.com --display-name Administrator
```

命令在同一数据库事务中创建管理员、分配 `super_admin` 并记录 Audit；不会生成默认账户或硬编码 ID。
