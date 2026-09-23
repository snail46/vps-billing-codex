# 17 — Deployment

V1：reverse-proxy、user-web、admin-web、server、worker、runman-gateway、postgres、redis。

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

Fake Payment 仅用于 V1 验收和非生产测试，回调使用 `FAKE_PAYMENT_WEBHOOK_SECRET` 的 HMAC-SHA256 签名。部署必须使用独立的至少 32 字符密钥。生产接入真实网关时必须新增独立 Adapter、验签与 Contract Test，不得复用 Fake secret。

Subscription lifecycle Worker 使用 `SUBSCRIPTION_GRACE_PERIOD`（Go duration，默认 `72h`）计算从计费周期结束开始的宽限期。修改该值只影响之后进入 past_due 的订阅；已持久化的 `grace_until` 不回溯修改。

Worker 需要访问 Redis Stream `operation-queue`（consumer group `operation-workers`）与 `domain-events`。PostgreSQL 是 Operation 的真相源；Redis 暂时不可用时 Outbox 保留待发布事件，恢复后继续投递。部署期间至少运行一个 Worker；失联 consumer 与 stale heartbeat 的自动恢复由 Phase 11 Reconciler 提供。

开发/验收环境可在 providers 表配置 `provider_type=mock`；Worker 的 Dynamic Registry 会按 ID 延迟创建 Mock Adapter。生产禁止使用 Mock，必须在 Phase 7/10 提供已核对官方协议的 Adapter Factory。启用销售的 Plan 必须配置 active NodeGroup、兼容能力和 default_image_id，否则 Provision 保持失败/重试状态而不会假成功。

Direct LXD 部署使用 `provider_type=lxdapi`。Provider `endpoint` 必须为受信任的 HTTPS LXD 地址，`credential_ref` 只允许大写字母、数字和下划线。若引用为 `LXD_PRIMARY`，Worker 环境必须提供 `LXD_PRIMARY_CLIENT_CERT_PEM`、`LXD_PRIMARY_CLIENT_KEY_PEM`、`LXD_PRIMARY_SERVER_CA_PEM`。Provider `config` 可包含 `project`、`image_server`、`operation_timeout_seconds`；证书和私钥禁止写入数据库或日志。Node `external_ref` 必须对应 LXD cluster member target。

Runman Gateway 默认监听 `RUNMAN_GATEWAY_ADDRESS=:9090`。生产必须同时配置 `RUNMAN_TLS_CERT_FILE` 与 `RUNMAN_TLS_KEY_FILE`；`RUNMAN_INSECURE=true` 只允许非生产本地/CI 环境。先创建 `provider_type=runman` 的 Provider 与 Node；该 Node 的 `provider_node_id` 必须留空（自动回退平台 Node UUID）或显式设置为同一个平台 Node UUID。随后执行 `/app/issue-agent-token --node-id <uuid>`；token 只显示一次，数据库只保存摘要。Agent 使用 `authorization: Bearer <token>` 主动连接 Gateway。负载均衡必须保持长连接；命令与连接状态以 PostgreSQL 为恢复源，因此 Gateway/Worker 重启不会丢失已提交命令。

Worker 每秒运行恢复扫描：2 分钟无 heartbeat 的 Operation 会按 retry budget 重排队；90 秒无 Agent heartbeat 的 Runman Node 标记 offline；终态 Operation 遗留的过期 Reservation 会原子释放；Instance observed state 会从 Provider 周期刷新。Redis 重启期间 Outbox 保持 pending，恢复后继续发布；Worker 崩溃遗留的 Redis Stream pending entry 会由其他 consumer 接管。生产告警应设置在这些恢复阈值之前，避免把自动恢复当作正常稳态。
