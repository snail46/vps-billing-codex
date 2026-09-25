# 11 — Admin UX

后台第一目标：快速发现和定位异常。

Dashboard 第一屏：Critical Alerts、Node Health、Failed Operations、Provider Health、Capacity Warnings；商业图表第二屏。

Instance Detail 必须显示 user/subscription/plan/provider/node/provider id/desired/observed/resources/network/traffic/operations/audit/raw diagnostic（权限控制）。

Operation Detail：steps、retries、error code、raw error、trace id、timestamps。

后台允许更高信息密度，但必须层级清晰。

V1 Admin Web 使用异常优先 Dashboard 与高密度资源表。导航按 RBAC permission 生成；直接访问无权限资源显示 permission denied。实例与 Operation 详情在具备额外管理权限时显示 raw diagnostics，否则服务端裁剪。用户状态、钱包 adjustment、工单回复/状态、角色和设置操作均即时显示成功/失败并触发审计。

V2 adds catalog lifecycle forms, secret-safe Provider health/inventory detail, Operation attempts/scheduler decisions with retry/cancel controls, usage/revenue diagnostics, dead-letter visibility/replay, and payment refunds. Dangerous actions are permission-gated, show server errors, and write Audit in the same transaction as the mutation.
