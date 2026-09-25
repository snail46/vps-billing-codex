# 16 — Observability

JSON structured logging。
标准：request_id、trace_id、actor、action、resource、operation_id、provider、node。

Trace：request→operation→workflow→provider task。

Metrics：
HTTP、queue、outbox、operation、workflow、provider、heartbeat、provision、payment callbacks、capacity。

Server 在受 Bearer token 保护的 `GET /metrics` 输出 Prometheus text format：按 method/status 的 HTTP 数量与耗时、pending Outbox、active/failed Operation、离线 Node、过期 Agent heartbeat、24 小时成功 Payment，以及在线 Node 剩余 CPU/Memory/Disk。`vps_billing_metrics_collection_success=0` 表示数据库采集失败，应立即告警。

告警阈值必须早于自动恢复：Operation heartbeat 120 秒、Agent heartbeat 90 秒；同时监控 Outbox 增长、5xx、失败 Operation、Provider/Node 离线、容量不足、备份失败与恢复演练失败。Admin Dashboard 提供面向人的异常视图，Prometheus 指标用于自动告警。

Audit 与普通日志分离。

Operation 对用户暴露稳定 error_code、message_key、trace_id 和步骤进度，不暴露 Provider 原始错误。SSE `Last-Event-ID` 用于断线续读，GET Operation 是权威恢复/轮询通道；SSE 仅转发与当前登录用户 user_id 匹配的事件。

V2 operational queries expose Provider health history, immutable Operation attempts, Scheduler decisions, usage periods, and dead-letter age/count without secrets. Alert on any dead letter, oldest pending Outbox age, usage collection lag, open rating periods past their end, Provider capability drift, expired Operation deadlines, and reservation pressure.
