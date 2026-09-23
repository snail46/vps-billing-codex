# 18 — Test Plan

Unit：价格、Ledger、状态机、权限、Scheduler。  
Integration：Postgres、Redis、Outbox、Payment、Provider Adapter。  
Provider Contract：health/capabilities/create/get/actions/traffic/NAT/idempotency/error normalization。  
E2E：注册→购买→支付→开通→操作→重装→流量→续费→暂停/恢复。  
Failure Injection：CREATE_TIMEOUT、CREATE_SUCCESS_BUT_TIMEOUT、NODE_OFFLINE、REINSTALL_FAIL、DELETE_NOT_FOUND、NETWORK_FAIL。

Scheduler Integration：相同输入排序确定、能力过滤、并发 Reservation 不超卖、Operation 幂等、commit/release 守恒。MockProvider Contract：重复 Create、同键冲突、完整动作、NAT/traffic/usage、unsupported error normalization。

Operation Integration：并发相同 idempotency key 只创建一个 Operation；Outbox→Redis Stream→Worker 首次可重试失败；RetryScheduler 到期重投；第二次执行成功；跨用户查询返回 not found；SSE 事件 ownership filter 拒绝无 owner 或其他用户事件。

Provision Integration：paid purchase event→pending Subscription/Instance/Operation→deterministic Scheduler Reservation→MockProvider Create/Get→running Instance→committed capacity→active Subscription→fulfilled Order→ready Notification；重复 Trigger 只保留一条 Operation。Compose Gate 必须通过真实 HTTP 支付回调并轮询授权 Operation API 看到 11 个步骤和 succeeded。

LXD Direct Provider Contract：使用协议级 HTTP fixture 验证 project/cluster target、异步 Operation wait、确定性 Create replay、动作/重装/删除 replay、同键跨实例冲突、状态/IP/流量映射、超时与 HTTP 错误标准化，以及 ResetPassword/NAT 的 `UNSUPPORTED_OPERATION`。测试构造器可使用本地 HTTP；数据库 Factory 永远要求 HTTPS+mTLS。

User Portal：验证中英文 key 集完全一致；前端 lint/typecheck/test/build；浏览器检查 desktop/mobile、loaded/error/empty、危险确认和语言切换。Compose 使用真实会话验证 Instance ownership 查询、network/traffic、notifications、Ticket 创建/回复，以及 Restart `202 → Operation → Worker → MockProvider → verify → succeeded`。数据库测试验证同一 Instance 只有一个 active action。

Admin Web：验证独立 Admin Session 与逐路由 RBAC；所有资源组均经真实 HTTP 查询。Wallet adjustment 集成测试与 Compose Gate 验证双分录相等、projection 同步及 Audit 同事务；secret setting 在写入和列表响应中始终遮罩。前端执行双语 parity、lint、typecheck、test、build 与浏览器 desktop/mobile 检查。

Runman Contract：使用真实生成的 gRPC client/server 与内存 transport 验证 Bearer auth、Heartbeat/VM 状态映射、message_id 去重、durable command 投递、CommandResult、断线后相同 command_id 重放；Adapter Contract 验证 Create/动作幂等、traffic/usage/IP 映射、NAT 与 offline 错误标准化。PostgreSQL 集成测试验证 token hash 鉴权、在线判定和 jsonb 规范化后的幂等比较。

Resilience Integration：stale running Operation 在 PostgreSQL 事务中重排队并生成新 Outbox；终态 Operation 的过期 Reservation 原子归还全部 reserved capacity；过期 Agent heartbeat 将 Node=offline、Instance=unknown；suspended desired/observed 漂移只创建 Suspend Operation。Redis 故障时 queued Outbox 保持 pending 并在新连接恢复后发布；模拟 Worker 在 XREADGROUP 后崩溃，replacement consumer 通过 XAUTOCLAIM 完成原 Operation。Provider response-loss 测试验证成功 Create 的同键重试仍只有一个 Instance。

Release Gate：验证生产配置 fail-closed、安全响应头、受保护 Metrics、全部 12 组 migration up/down 配对、部署脚本语法、npm audit、Admin TOTP 服务与双语 UI，以及完整 backend race test、frontend lint/test/typecheck/build、Compose 购买/支付/开通/实例动作/Admin Audit 流程。Migration 12 为 Outbox、失败 Operation、成功 Payment、Audit、Ticket 与未读 Notification 热路径提供索引。
