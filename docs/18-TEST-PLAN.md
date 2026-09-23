# 18 — Test Plan

Unit：价格、Ledger、状态机、权限、Scheduler。  
Integration：Postgres、Redis、Outbox、Payment、Provider Adapter。  
Provider Contract：health/capabilities/create/get/actions/traffic/NAT/idempotency/error normalization。  
E2E：注册→购买→支付→开通→操作→重装→流量→续费→暂停/恢复。  
Failure Injection：CREATE_TIMEOUT、CREATE_SUCCESS_BUT_TIMEOUT、NODE_OFFLINE、REINSTALL_FAIL、DELETE_NOT_FOUND、NETWORK_FAIL。

Scheduler Integration：相同输入排序确定、能力过滤、并发 Reservation 不超卖、Operation 幂等、commit/release 守恒。MockProvider Contract：重复 Create、同键冲突、完整动作、NAT/traffic/usage、unsupported error normalization。

Operation Integration：并发相同 idempotency key 只创建一个 Operation；Outbox→Redis Stream→Worker 首次可重试失败；RetryScheduler 到期重投；第二次执行成功；跨用户查询返回 not found；SSE 事件 ownership filter 拒绝无 owner 或其他用户事件。
