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

Worker heartbeat 超过 2 分钟未更新时，Reconciler 将未终态 Operation 在 retry budget 内重新置为 queued，并在同一事务写入新的 `operation.queued.v1` Outbox；超出 budget 则显式 failed。Agent heartbeat 超过 90 秒时 Node 进入 offline，其非终态 Instance observed_state 进入 unknown，禁止推断为 deleted。Heartbeat 恢复后 Agent 可重新把 Node/Instance observed state 收敛。

Subscription suspended 只推动 Instance desired_state=suspended；Reconciler 创建 `suspend` Operation，由 Workflow 调用 Provider Stop 并验证后才写 observed_state=suspended。续费恢复 active 时，仅将先前 suspended 的 desired_state 改回 running，再创建 Start Operation。

Subscription 时间迁移：active 在 period end 后进入 past_due 并使用 `period_end + grace_period`；past_due 在 grace deadline 后进入 suspended；设置 `cancel_at_period_end` 的 active 在 period end 进入 cancelled。成功续费可将 active/past_due/suspended 置回 active 并清除 grace/cancel 标记；终态拒绝续费。

首次购买：Order paid→fulfilling→fulfilled；Subscription pending→active；Instance observed pending→provisioning→running。只有 Provider 验证 running 且 Reservation 原子转 committed 后才能激活 Subscription。失败保持 Operation error_code/trace；Provider 非重试错误将 Instance 标为 error 并释放仍为 reserved 的容量。
