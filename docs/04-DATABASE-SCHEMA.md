# 04 — Database Schema

数据库 PostgreSQL；参考 `db/schema.sql`。

规则：
- 核心 ID UUIDv7（应用层生成可接受）
- 时间 TIMESTAMPTZ/UTC
- 金额 BIGINT minor unit
- 财务数据不物理删除
- Payment+Order+Ledger+Outbox 同事务
- Reservation 使用 DB 行锁再次检查容量
- payments/operations 使用唯一 idempotency key

核心表见 schema.sql。

Identity 使用相互独立的 `user_sessions` 与 `admin_sessions`。数据库仅保存经各自密钥 HMAC-SHA256 后的 opaque session token；会话支持过期、访问时间和撤销时间。角色与权限由版本 migration 初始化，管理员身份不允许从用户会话提升。

Commerce 使用用户范围的订单幂等键、网关事件唯一回执、Payment 外部 ID 唯一约束和 Ledger 引用唯一约束。Ledger transaction/entry 由数据库 trigger 禁止 UPDATE/DELETE。支付成功时 Payment、Order、Invoice、双分录 Ledger 和 Outbox 必须在同一事务提交。

Subscription 续费通过 `orders.kind=renewal` 与 `orders.subscription_id` 明确关联，仍创建独立 Order / Invoice / Payment。续费支付事务同时延长 Subscription 并写 `subscription.renewed.v1`；重复网关事件不能重复延长周期。生命周期截止时间建立索引，由 Worker 以行锁批量推进。

Infrastructure 的节点容量同时记录 total / allocated / reserved，覆盖 CPU、内存、磁盘、IPv4、IPv6 和 NAT 端口。Scheduler 在事务中锁定候选 Node、再次检查容量、增加 reserved 并插入每 Operation 唯一的 Reservation；数据库约束禁止负数与超卖。commit 将 reserved 原子转为 allocated，release 原子归还 reserved。
