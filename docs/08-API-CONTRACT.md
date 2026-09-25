# 08 — API Contract

Base `/api/v1`。OpenAPI 见 `docs/openapi/openapi.yaml`。

成功：
`{success:true,data:{},request_id}`

失败：
`{success:false,error:{code,message_key,details},request_id}`

异步修改动作：HTTP 202 + operation_id。

HTTP：
200/201/202/400/401/403/404/409/422/429/500/502/503。

用户 API：Auth、Products、Orders、Payments、Subscriptions、Instances、Operations、Wallet、Invoices、Notifications、Tickets。  
Admin API：`/api/v1/admin/*`。

Operation 查询必须校验 owner；不存在和非 owner 均返回 404。`GET /events` 使用登录会话建立 SSE，支持 `Last-Event-ID`，只发送当前用户事件。客户端必须在断线、页面刷新或 SSE 不可用时轮询 Operation 查询恢复状态。

价格永远服务端计算。

Subscription：`GET /subscriptions`；`POST /subscriptions/{id}/renewals` 使用 Idempotency-Key 创建续费 Order/Invoice/Payment；`PUT /subscriptions/{id}/cancel-at-period-end` 安排或撤销周期末取消。所有 ownership 由服务端会话校验。

User Portal：`GET /instances`、`GET /instances/{id}`、`GET /instances/{id}/networks|traffic`、Notifications、Tickets 均在查询层校验 user ownership。Start/Stop/Restart/Reinstall 必须带 CSRF 与 Idempotency-Key，返回 `202 + operation_id`；同一 Instance 同时只允许一个未终态动作。Ticket 创建与首条消息同事务写入，关闭/解决后的 Ticket 禁止追加消息。

Admin Control Plane：`/admin/*` 使用独立 Admin Session、CSRF 与逐 Handler permission。列表覆盖 User/Product/Order/Payment/Ledger/Subscription/Instance/Node/Provider/Operation/Ticket/Audit/Admin/Role/Setting；secret 与 credential 永不返回。余额调整只能创建平衡 Ledger adjustment；用户状态、工单、设置与角色修改必须与 Audit 同事务。

内部运维端点 `GET /metrics` 不属于 `/api/v1`，返回 Prometheus text format，并要求 `Authorization: Bearer <METRICS_TOKEN>`。Reverse Proxy 默认不公开该路径；监控系统应从受限管理网络直接抓取 Server。

V2 adds capability/availability fields to catalog responses; owned instance usage and NAT endpoints; billing-profile GET/PUT; product/plan management; provider detail; operation retry/cancel; usage and dead-letter lists; audited dead-letter replay; and idempotent refund creation. Order creation accepts optional `promotion_code`. The authoritative machine-readable contract is `docs/openapi/openapi.yaml` version 2.0.0.
