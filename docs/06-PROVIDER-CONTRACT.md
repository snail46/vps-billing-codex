# 06 — Provider Contract

业务层不理解第三方专有 API。

统一接口：
Health、Capabilities、ListImages、Create/Get、Start/Stop/Restart/Reinstall/ResetPassword/Delete、Usage/Traffic、PortForward。

Capability 驱动 UI/业务，禁止散落 provider 类型判断。

统一错误：
PROVIDER_TIMEOUT、PROVIDER_UNAVAILABLE、PROVIDER_AUTH_FAILED、NODE_OFFLINE、RESOURCE_EXHAUSTED、IMAGE_NOT_FOUND、INSTANCE_NOT_FOUND、INSTANCE_ALREADY_EXISTS、PORT_EXHAUSTED、NETWORK_ERROR、UNSUPPORTED_OPERATION、UNKNOWN_PROVIDER_ERROR。

Create 必须幂等；timeout 后 Verify。

Direct Provider：LXDAPI/CLICD 等。  
Agent Provider：Runman Gateway → runman-agent。

实现前核对第三方当前官方文档/源码；只改 Adapter，不改 Business Contract。

V1 MockProvider 是完整的内存 Contract 实现：覆盖 health/capabilities/images/create/get/actions/usage/traffic/NAT，Create 与动作均要求幂等键，且同键不同请求会显式失败。它只用于自动化验收和开发，不作为生产 Provider。Provider Registry 按数据库 Provider ID 解析 Adapter，业务代码不按 provider type 分支。

Worker 使用 Dynamic Registry 按数据库 Provider ID 延迟解析 Adapter；provider_type→Factory 的绑定只允许出现在 composition/provider layer。Mock Create 返回可重复查询的 running 实例，并按请求生成文档保留网段地址，供 Provision 网络持久化测试。

V1 Direct Provider 为 `lxdapi`。它通过 Canonical LXD `/1.0` REST API 实现 Health、Capabilities、Images、Create/Get、Start/Stop/Restart/Reinstall/Delete、Usage/Traffic，并将异步 LXD Operation 等待限制在配置超时内。Create 使用确定性实例名和 LXD 实例配置中的平台身份验证实现跨进程安全重试；动作幂等键在同一 Adapter 生命周期内拒绝跨实例复用。ResetPassword 与 NAT 明确标记不支持。

Provider 数据库配置示例（不包含 Secret）：`endpoint=https://lxd.example:8443`、`credential_ref=LXD_PRIMARY`、`config={"project":"billing","image_server":"https://cloud-images.ubuntu.com/releases","operation_timeout_seconds":30}`。业务和 Workflow 只依赖 Provider Contract，不读取这些字段。

V1 Agent Provider 为 `runman`。独立 gRPC Gateway 接受由 Agent 发起的双向流，使用 Bearer Token 的 SHA-256 摘要鉴权；生产传输强制 TLS。Heartbeat 更新 Node/VM observed state、流量和镜像目录；Worker 仅向数据库写入带唯一 idempotency key 的 durable command，Gateway 对在线连接投递并在重连时以相同 command_id 重放 queued/dispatched command。Agent message_id 全局去重，CommandResult 为终态；NAT 列表使用协议专用 PortForwardList 回执。业务层仍只依赖本 Contract。

Runman Create 使用平台 Instance UUID 作为协议 vm_id，因此“执行成功但响应丢失”重试不会创建第二台 VM。Reinstall 所需 CPU/RAM/Disk/Bandwidth 从平台 Instance 商业真相读取，不从 Agent 推断。协议错误统一映射为 NODE_OFFLINE、PROVIDER_TIMEOUT、INSTANCE_NOT_FOUND 或 UNKNOWN_PROVIDER_ERROR；不支持的能力仍必须返回 UNSUPPORTED_OPERATION。
