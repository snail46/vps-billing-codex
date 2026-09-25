# 10 — User UX

用户不关心 Provider/Agent/Workflow。

页面：
dashboard、products、checkout、instances/detail/network/traffic/activity、orders、invoices、wallet、notifications、tickets、account/security。

Dashboard 5 秒内回答：服务器数量/运行/需处理/余额/快到期/流量预警。

Instance Detail：状态、OS、IP/SSH、CPU/RAM/Disk、流量、到期、Start/Stop/Restart/Reinstall/Renew。

异步操作必须即时反馈、自动进度、刷新后恢复 Operation。

所有页面必须考虑 loading/loaded/empty/error/partial_error/permission_denied。

V1 User Web 已实现响应式导航、Dashboard 指标、Catalog/Checkout、Instance 详情/网络/流量/动作、Operation 实时进度与轮询恢复、Orders、Invoices、Wallet、Notifications、Tickets 及 Account。危险的 Reinstall 必须二次确认并说明数据丢失后果。Dashboard 和 Instance Detail 在部分请求失败时保留其余可用数据并展示 partial-error Alert。

V2 adds VPS/NAT VPS and region/availability presentation, first-class Subscriptions, renewal/cancel-at-period-end, NAT mapping management, current allowance/overage estimates, and editable billing profile. NAT actions display the returned Operation immediately and retain polling recovery. All added strings have zh-CN/en-US parity.
