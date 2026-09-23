# 16 — Observability

JSON structured logging。
标准：request_id、trace_id、actor、action、resource、operation_id、provider、node。

Trace：request→operation→workflow→provider task。

Metrics：
HTTP、queue、outbox、operation、workflow、provider、heartbeat、provision、payment callbacks、capacity。

Audit 与普通日志分离。

Operation 对用户暴露稳定 error_code、message_key、trace_id 和步骤进度，不暴露 Provider 原始错误。SSE `Last-Event-ID` 用于断线续读，GET Operation 是权威恢复/轮询通道；SSE 仅转发与当前登录用户 user_id 匹配的事件。
