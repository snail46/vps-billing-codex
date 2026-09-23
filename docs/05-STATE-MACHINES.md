# 05 — State Machines

Order: pending→paid→fulfilling→fulfilled；pending→cancelled；paid→refund_pending→refunded  
Payment: pending/processing/succeeded/failed/partially_refunded/refunded  
Subscription: pending/active/past_due/suspended/cancelled/expired/terminated  
Node: online/degraded/draining/maintenance/offline  
Desired Instance: running/stopped/suspended/deleted  
Observed Instance: pending/provisioning/running/stopping/stopped/restarting/reinstalling/suspending/suspended/deleting/deleted/error/unknown  
Operation: queued/running/waiting_provider/waiting_resource/verifying/retrying/succeeded/failed/cancelled

Operation 只有 queued 可被 Worker 原子 claim。可重试失败进入 retrying 并持久化 next_attempt_at；RetryScheduler 到期后生成新的 queued Outbox 事件。终态不可由进度、步骤或重复队列消息重新打开。OperationStep: pending/running/waiting/succeeded/failed/skipped。

Provider 不可达时实例为 unknown，不得猜 stopped/deleted。
Create timeout 先 verifying，不得盲目重复创建。

Subscription 时间迁移：active 在 period end 后进入 past_due 并使用 `period_end + grace_period`；past_due 在 grace deadline 后进入 suspended；设置 `cancel_at_period_end` 的 active 在 period end 进入 cancelled。成功续费可将 active/past_due/suspended 置回 active 并清除 grace/cancel 标记；终态拒绝续费。

首次购买：Order paid→fulfilling→fulfilled；Subscription pending→active；Instance observed pending→provisioning→running。只有 Provider 验证 running 且 Reservation 原子转 committed 后才能激活 Subscription。失败保持 Operation error_code/trace；Provider 非重试错误将 Instance 标为 error 并释放仍为 reserved 的容量。
