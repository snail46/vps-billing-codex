你现在负责将当前仓库从现有状态持续施工到完整 V1 发布候选版本。

这不是单阶段任务。

不要在完成某一个 Phase 后停止，不要等待我逐阶段确认，不要因为 TASKS.md 中一个阶段完成就结束任务。

你的最终停止条件只有：

V1 全部 Phase 完成
+
全部验收通过
+
关键测试通过
+
项目可以按照文档部署运行
+
没有已知的 P0/P1 阻塞问题。

==================================================
一、开始前必须重新读取项目规范
==================================================

在修改任何代码之前，重新阅读并遵守：

AGENTS.md
MASTER_PROMPT.md
TASKS.md

以及 docs/ 下全部工程规范，尤其：

01-PRD.md
02-ARCHITECTURE.md
03-DOMAIN-MODEL.md
04-DATABASE-SCHEMA.md
05-STATE-MACHINES.md
06-PROVIDER-CONTRACT.md
07-WORKFLOW-SPEC.md
08-API-CONTRACT.md
09-EVENT-CONTRACT.md
10-USER-UX.md
11-ADMIN-UX.md
12-DESIGN-SYSTEM.md
13-I18N-SPEC.md
14-SECURITY.md
15-RBAC.md
16-OBSERVABILITY.md
17-DEPLOYMENT.md
18-TEST-PLAN.md
19-ACCEPTANCE-CRITERIA.md
20-AI-IMPLEMENTATION-RULES.md

同时阅读：

docs/openapi/
docs/schemas/
docs/adr/

以上文件是架构和施工契约，不得为了方便而绕过。

==================================================
二、当前已经确定的技术栈不得擅自改变
==================================================

后端：

Go
chi
pgx/v5
sqlc
golang-migrate

架构：

Modular Monolith

数据库：

PostgreSQL

缓存 / Queue：

Redis

前端：

Vite
React
TypeScript
Tailwind CSS
TanStack Query
i18next

前端应用：

user-web
admin-web

语言：

zh-CN
en-US

除非发现无法施工的根本性技术冲突，否则禁止更换技术栈。

任何架构级修改必须新增 ADR，并明确说明：

为什么修改
影响范围
迁移方案
兼容性
测试方式

禁止引入 GORM 或 ORM AutoMigrate。

==================================================
三、首先验收现有 Phase 0
==================================================

当前 Phase 0 已经有人施工过。

不要假设它一定正确。

先全面检查：

项目目录结构
Go Module
server
worker
user-web
admin-web
PostgreSQL
Redis
Docker Compose
Migration
sqlc
i18n
共享 UI
CI
ADR-001
ADR-002
lint
test
build

如果发现缺失、临时实现、编译失败、测试失败或规格不符：

立即修复。

Phase 0 验收通过后：

不要停止。

立即进入 Phase 1。

==================================================
四、持续完成完整 V1
==================================================

严格按照 TASKS.md 的阶段顺序持续执行：

Phase 0
工程骨架与基础设施

Phase 1
Identity

Phase 2
Commerce

Phase 3
Subscription

Phase 4
Infrastructure Domain

Phase 5
Operation System

Phase 6
Provision Workflow

Phase 7
Direct Provider

Phase 8
User Web

Phase 9
Admin Web

Phase 10
Runman

Phase 11
Resilience

Phase 12
Release Hardening

每完成一个 Phase：

1. 按该 Phase 的 Definition of Done 自检
2. 运行该阶段相关测试
3. 修复所有发现的问题
4. 更新 TASKS.md
5. 更新相关 docs
6. 必要时新增 Migration
7. 必要时更新 OpenAPI
8. 必要时更新状态 Registry
9. 必要时新增 ADR
10. 自动进入下一个 Phase

不要停下来向我询问“是否继续”。

==================================================
五、允许你自主做出的决定
==================================================

对于不会改变核心架构的小型工程决策，你可以自行选择合理方案。

例如：

包目录细节
函数名称
内部 helper
测试 fixture
组件拆分
CSS 细节
小型库选型
日志实现细节
Mock 实现方式

优先采用：

简单
成熟
稳定
依赖少
容易测试
容易维护

的方案。

不要因为这种普通工程选择向我提问。

==================================================
六、只有以下情况允许暂停并询问
==================================================

只有真正无法继续施工时才允许询问，例如：

缺失必须由我提供的真实 API Secret

需要真实支付商户凭据才能继续生产集成

需要访问一个当前无法获得的私有仓库

第三方 Provider 官方文档与源码存在无法判断的重大冲突

两个不可兼容方案会永久改变公开 API / 数据模型，而现有规范没有给出答案

涉及不可逆的数据删除或外部生产操作

除此之外：

请自行分析、实现、测试、修复、继续。

不要因为：

测试失败
编译错误
类型错误
Migration 错误
Lint 错误
Provider Mock 错误
前端报错

而停止。

这些属于你应该自行解决的问题。

==================================================
七、禁止为了“完成任务”做假实现
==================================================

禁止：

返回假成功

用 TODO 代替 V1 核心能力

用静态 Mock 数据冒充真实业务实现

吞掉错误

catch 后什么都不做

注释掉失败测试

删除测试让 CI 通过

绕过 RBAC

绕过 Ledger

绕过 Operation

绕过 Workflow

绕过 Provider SDK

绕过 Reconciler

直接修改钱包余额

Handler 直接写 SQL

Handler 直接调用 Provider

前端直接调用 Provider

前端自己推断服务器真实状态

写死 Node ID

写死 User ID

写死 Provider

硬编码中文或英文 UI 文案

为了通过构建而删掉核心功能

==================================================
八、核心架构边界必须保持
==================================================

Business Core 是唯一商业真相源。

Order != Payment != Subscription != Instance。

Wallet Balance 是 Ledger Projection。

Ledger 历史记录不可直接修改。

Instance 必须同时维护：

desired_state
observed_state

所有长时间操作必须：

HTTP 接收请求
→ 创建 Operation
→ Queue
→ Worker
→ Workflow
→ Provider
→ 自动更新 Operation
→ SSE / Polling 通知前端

所有第三方虚拟化后端必须通过 Provider SDK。

Business Core 不允许知道：

CLICD 特有字段
LXDAPI 特有字段
Runman Command 名称
Proxmox 特有字段

Provider Adapter 负责：

参数转换
状态转换
错误转换
Capability
幂等
Provider API 调用

==================================================
九、Provider 开发规则
==================================================

先保证 MockProvider 完整。

然后实现至少：

1 个 Direct Provider

优先根据仓库现有设计实现：

CLICDProvider
或
LXDAPIProvider

之后实现：

RunmanProvider
+
Runman Gateway

Runman 属于 Agent Provider，不能强行按 HTTP Direct Provider 模式实现。

如果第三方接口与当前认知不一致：

检查官方文档和源码。

不要猜 API。

如果某能力第三方确实不支持：

Capabilities 对应字段 = false。

不要修改整个 Business Core 来迁就某个 Provider。

==================================================
十、用户前台必须按用户视角开发
==================================================

用户首先关心：

服务器是否正常
怎么连接
流量还有多少
什么时候到期
怎么续费
操作是否成功
重装是否完成

不要向普通用户暴露：

Provider UUID
Agent Command ID
Workflow Internal ID
Raw Provider Error

用户所有长任务必须有明确反馈。

例如重装：

已提交
正在停止
正在重装
正在配置网络
正在启动
正在验证
完成

页面无需手动刷新。

失败必须显示：

可理解的失败原因
是否可以重试
错误编号
联系支持入口

==================================================
十一、Admin 必须按管理员 / 运维视角开发
==================================================

后台优先回答：

系统是否健康
哪些节点异常
哪些任务失败
哪个 Provider 出错
容量是否不足
用户是否付款
Ledger 是否正确
实例是否真的存在
失败卡在哪一步

管理员应可以从 Operation Detail 看到：

Workflow Steps
Node
Provider
Provider Operation ID
Retry Count
Error Code
Raw Error
Trace ID

但必须受 RBAC 控制。

==================================================
十二、双语和 UI 规范
==================================================

整个 UI 必须支持：

zh-CN
en-US

禁止硬编码 UI 文案。

必须通过 i18n Key。

用户前台和管理后台可以共享基础 UI Package，但布局不得强行相同。

User Web：

简单
直观
低认知成本

Admin Web：

高信息密度
运维效率
异常优先

所有页面必须实现：

Loading
Loaded
Empty
Error
Partial Error
Permission Denied

禁止 API 报错后页面无提示。

==================================================
十三、测试要求
==================================================

开发过程中持续执行：

Go fmt
Go lint
Go unit tests
Go integration tests

frontend typecheck
frontend lint
frontend tests
frontend build

migration validation

provider contract tests

E2E

Failure Injection

每一个 Phase 完成后：

先修复测试再进入下一阶段。

不要积累大量失败到最后一起处理。

==================================================
十四、必须完成的关键自动化测试
==================================================

至少覆盖：

用户注册

用户登录

管理员登录

User / Admin Session 隔离

RBAC

商品

套餐

创建订单

支付成功

重复支付 callback

Wallet

Double-entry Ledger

Subscription

续费

过期

Grace Period

Suspend

Unsuspend

Node Scheduler

Resource Reservation

Operation

Workflow

SSE

MockProvider

真实 Direct Provider Adapter

Runman Agent Connect

Runman Heartbeat

Runman Command

Runman CommandResult

Create Instance

Restart Instance

Reinstall Instance

Delete Instance

NAT Port

Traffic

Provider Timeout

Create Success But Timeout

Worker Crash

Redis Restart

Node Offline

Agent Disconnect

Duplicate Create Request

Reconciler Recovery

==================================================
十五、特别测试以下高风险场景
==================================================

100 次相同 Payment Callback：

只能产生：

1 次成功付款
1 次 Ledger 资金变化
1 次 Order Paid
1 次 Provision Workflow

Provider Create 返回成功但响应丢失：

不能创建第二台实例。

Worker 创建 VPS 中途崩溃：

重启后必须可以恢复。

Redis 暂时宕机：

已提交商业事务不能永久丢任务。

Agent 离线：

Node = offline
Instance = unknown

不能把实例误判为 deleted。

Reinstall 进行中：

重复点击不得产生两个重装任务。

管理员修改余额：

必须产生 Ledger Transaction
+
Audit Event。

==================================================
十六、代码质量要求
==================================================

代码必须：

可读
可测试
模块边界明确
错误显式处理
日志结构化
避免循环依赖
避免超大文件
避免超大函数

不要过度抽象。

不要为了所谓“企业级”引入：

Kubernetes
Kafka
Service Mesh
复杂 CQRS
复杂 Event Sourcing
几十个微服务

除非现有 V1 规范明确要求。

==================================================
十七、持续维护文档
==================================================

代码和文档必须同步。

如果实现导致以下内容变化：

数据库
API
状态
事件
Provider
权限
部署方式

必须同步修改相关：

docs/
OpenAPI
Migration
status-registry
ADR

禁止出现：

代码已经改变
文档仍然描述旧行为

==================================================
十八、每个 Phase 建议建立内部 Checkpoint
==================================================

如果 Git 环境允许：

每完成一个 Phase 并通过测试后创建清晰 Commit。

例如：

phase1: complete identity and rbac

phase2: complete commerce and ledger

phase3: complete subscription lifecycle

...

不要将整个 V1 做成一个巨大 commit。

不要 push 到远程，除非当前环境明确授权。

==================================================
十九、最终 V1 完成条件
==================================================

只有下面全部满足才允许停止：

TASKS.md 中 V1 项目全部完成

Phase 0-12 全部验收通过

核心 Migration 完成

OpenAPI 与实际实现一致

zh-CN / en-US 完整

User Web 可以正常完成核心用户生命周期

Admin Web 可以正常完成核心管理生命周期

Provider Contract Tests 通过

MockProvider 通过

Direct Provider 能正常工作

Runman Provider / Gateway 按规范完成

关键 Integration Tests 通过

关键 E2E Tests 通过

Failure Tests 通过

Docker Compose 可以完成部署

健康检查正常

没有已知 P0/P1 缺陷

没有 silent failure

没有重复收费问题

没有重复创建 VPS 问题

没有权限越权问题

没有永久丢任务问题

==================================================
二十、完成后进行一次全项目 Final Audit
==================================================

Phase 12 完成后不要立即结束。

执行最终审计：

1. 搜索 TODO / FIXME / HACK

2. 搜索硬编码中文和英文 UI

3. 搜索 Handler 直接 SQL

4. 搜索 Handler 直接 Provider 调用

5. 搜索未处理 error

6. 搜索空 catch

7. 检查 RBAC

8. 检查支付幂等

9. 检查 Ledger 平衡

10. 检查 Outbox

11. 检查 Operation Recovery

12. 检查 Reconciler

13. 检查 Provider Capabilities

14. 检查所有 Migration

15. 检查 OpenAPI

16. 检查双语

17. 检查 Docker Compose

18. 执行完整 lint/test/build/E2E

发现问题自行修复。

修复后重新运行完整验证。

==================================================
二十一、最终才向我汇报
==================================================

最终完成 V1 后，汇报：

V1 完成情况

Phase 0-12 状态

架构实现情况

数据库 Migration

API 实现情况

Provider 实现情况

User Web 完成情况

Admin Web 完成情况

测试数量及结果

E2E 结果

Failure Test 结果

安全检查结果

已知非阻塞问题

明确延期到 V1.1 / V2 的功能

启动方法

部署方法

默认管理员初始化方法

真实 Provider 配置方式

==================================================

现在开始。

首先验收已有 Phase 0。

发现问题就修复。

然后自动进入 Phase 1。

此后持续推进 Phase 2、3、4……直到 Phase 12 和完整 V1 Acceptance 全部通过。

不要在 Phase 之间停止等待我的确认。