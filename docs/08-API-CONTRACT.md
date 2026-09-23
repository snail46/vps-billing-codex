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

价格永远服务端计算。

Subscription：`GET /subscriptions`；`POST /subscriptions/{id}/renewals` 使用 Idempotency-Key 创建续费 Order/Invoice/Payment；`PUT /subscriptions/{id}/cancel-at-period-end` 安排或撤销周期末取消。所有 ownership 由服务端会话校验。
