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
