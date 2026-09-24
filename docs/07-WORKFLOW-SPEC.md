# 07 — Workflow Spec

Provision：
validate_subscription → select_node → reserve_resources → create_instance → wait_provider → configure_network → verify_running → persist_network → commit_reservation → activate_subscription → notify → finish。

Reinstall：
validate → lock → submit → wait → verify → sync_network → update_image → unlock → notify。

Delete：
validate → desired=deleted → provider_delete → verify_not_found → release network/ports/capacity → observed=deleted → audit。

Renew：
invoice → payment → ledger → extend subscription → update due → if suspended set desired running → reconcile。

Phase 3 完成 invoice/payment/ledger/extend/update due 的商业事务边界；涉及 Instance desired state 的恢复必须等待 Operation/Workflow 基础设施完成后执行，不允许在生命周期 Worker 中直接调用 Provider。

进度按步骤映射，不按时间伪造。

所有 Workflow 通过 Registry 按 Operation type 注册。创建 Operation、步骤和 `operation.queued.v1` Outbox 必须同事务；Outbox 将事件发布到 `domain-events`，并将 queued 事件投递到 Redis Stream `operation-queue`。Worker 使用 consumer group 原子 claim 数据库状态；失败按受限指数退避持久化重试。重复消息因 queued-only claim 成为 no-op。

Phase 6 的 `payment.succeeded.v1` Trigger 只处理 purchase Order，并以 `(source_order_id, source_item_index)` 幂等创建 pending Subscription、Instance、Provision Operation、Steps 与 queued Outbox；renewal 不触发开通。Provision 使用 Operation ID 作为 Scheduler/Provider 幂等根，按真实步骤更新进度，不按时间伪造。最终化事务提交 Reservation、激活 Subscription、完成全部实例均已 active 的 Order、创建 ready Notification 和版本化事件。

Phase 11 Reconciler 是 Worker Processor，不直接执行实例变更。它刷新 Provider observed state，并将 running/stopped/suspended 漂移转换为带幂等键的 Start/Stop/Suspend Operation，继续走相同 Workflow。stale Operation 的重排队和 Outbox 同事务；终态 Operation 的过期 Reservation 才能释放，避免仍在执行的 Workflow 被回收容量。Redis consumer pending message 由 replacement Worker 使用 XAUTOCLAIM 接管。
