# 15 — RBAC

角色：super_admin、operations、finance、support、read_only。

Permission 示例：
users.read/update/suspend
instances.read/start/stop/restart/reinstall/delete
payments.read/refund
ledger.read/adjust
nodes.read/update/delete
providers.read/manage
operations.read/retry
tickets.read/reply/manage
audit.read
admins.manage
roles.manage
settings.manage

每个 Admin Handler 显式声明权限。

Admin Web 额外声明：health.read、products.read/manage、orders.read、subscriptions.read、admins.read、roles.read、settings.read。Migration 000009 将 read 权限授予对应职责角色；super_admin 拥有全部权限。原始 Provider/Operation 诊断只对 manage/retry 权限开放。
