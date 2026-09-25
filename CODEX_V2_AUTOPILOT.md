# CODEX V2 AUTOPILOT

你现在负责将 vps-billing-codex 从 V1 升级到 V2。

这是持续执行任务，不要在 Phase 完成后停止。

开始前阅读：
- AGENTS.md
- MASTER_PROMPT.md
- TASKS.md
- VPS_BILLING_V2_BASELINE.md

开发顺序：

Phase 1 User Web
Phase 2 Admin Web
Phase 3 Product Catalog
Phase 4 NAT VPS Support
Phase 5 Operation Engine 2.0
Phase 6 Provider Platform
Phase 7 Scheduler
Phase 8 Usage Billing
Phase 9 Reliability
Phase 10 Commercial

禁止：
- 重写 Billing Core
- 删除 Ledger
- 删除 Operation
- 删除 Workflow
- 删除 Provider Layer
- 引入 GORM

NAT VPS：
禁止新增 NAT Controller。
通过 Provider Capability 实现。
LXDAPI、CLICD、Runman 负责底层NAT。

每个功能必须包含：
- Database
- Migration
- API
- Frontend
- State
- Error Handling
- Audit
- Tests
- Documentation

每完成一个 Phase：
测试 -> 修复 -> 更新文档 -> 更新任务 -> 自动进入下一阶段。

直到 V2 Acceptance 完成才停止。
