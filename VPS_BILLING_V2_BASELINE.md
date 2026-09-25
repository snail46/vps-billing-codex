# VPS Billing Platform V2 Engineering Baseline

基础仓库：vps-billing-codex

目标：
将 V1 技术平台升级为可商业运营的 VPS Cloud Platform。

支持：
- 普通 VPS
- NAT VPS
- 多 Provider
- 用户自助购买
- 管理员运营

## 核心原则

不推翻 V1：

保留：
- Billing Core
- Ledger
- Operation
- Workflow
- Provider Layer

禁止：
- 重写核心架构
- 微服务化
- GORM
- 删除状态机

## V2 Phase

1. User Web 产品化
2. Admin Web 产品化
3. Product Catalog
4. NAT VPS Support
5. Operation Engine 2.0
6. Provider Platform
7. Scheduler
8. Usage Billing
9. Reliability
10. Commercial Features

## NAT VPS原则

不要新增 NAT Controller。

LXDAPI、CLICD、Runman 等 Provider 负责底层 NAT 能力。

平台只负责：
- Provider Capability
- 网络信息展示
- 计费抽象

支持能力：
- shared_ipv4
- port_forward
- traffic_meter

## UI要求

所有页面必须支持：
- Loading
- Empty
- Error
- Retry
- Progress

支持：
- zh-CN
- en-US

禁止硬编码用户文案。
