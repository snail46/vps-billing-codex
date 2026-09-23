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
