# VPS Billing V2

VPS Billing V2 是一个面向 VPS 销售、交付、NAT 端口转发和自动化计费的生产级单体平台。
后端采用 Go 1.24+、PostgreSQL 17 与 Redis 8；前端包含面向最终用户的 **User Web** 与面向运维管理员的 **Admin Web**，全面基于 React 19、TypeScript 与 `@tanstack/react-query`，100% 接入真实后端 API 与状态机，支持 `zh-CN` 与 `en-US` 严格双语对齐。

---

## V2 核心特性与架构升级

### 1. 用户服务中心 (User Web)
- **概览面板 (Dashboard)**：实时呈现 VPS 总量、运行状态、待处理项、钱包余额、临期订阅和未读通知；提供进行中异步任务的全局横幅告警、快捷操作入口及最近服务器与订单列表。
- **套餐选购与结算 (Catalog & Checkout)**：支持独享公网 VPS 与 NAT VPS 分类展示及能力标签（IPv4 / IPv6 / NAT 转发端口 / 带宽）；下单流程支持数量调整、优惠码抵扣与明细拆解，通过 `POST /api/v1/portal/orders` 直连下单。
- **实例管理与控制台 (Instances & Detail)**：
  - 关键词搜索与运行状态筛选。
  - 一键复制 IP 与 MAC 地址，完整规格概览。
  - 实时流量使用率进度条展示与历史计费周期明细。
  - **NAT 端口转发全生命周期**：查看转发规则、新增外部端口映射 (`POST`)、删除端口规则 (`DELETE`)，接入 202 异步任务。
  - **电源与系统操作**：开机、关机、重启、重装操作系统（支持选择镜像），均配有二次防误触确认。
  - **实时运维进度跟踪**：集成 `OperationProgress`，基于 SSE `/api/v1/events` 实时推送步骤日志，具备 2s 轮询自动降级容灾。
- **订阅生命周期 (Subscriptions)**：展示计费周期与到期时间，支持手动提前发起续费订单 (`POST /api/v1/subscriptions/{id}/renewals`) 及到期自动取消开关 (`PUT /api/v1/subscriptions/{id}/cancel-at-period-end`)。
- **订单与发票 (Orders & Invoices)**：支持按状态筛选，提供结构化订单明细、支付网关跳转引导及关联合同发票。
- **钱包与账务 (Wallet)**：真实可用余额展示，底层依托双分录复式记账账本一致性保障。
- **工单支持中心 (Tickets)**：工单创建模态框、优先级状态跟踪、客服对话线程展示（自动区分用户与管理员发言）、结单回复保护。
- **账户与开票资料 (Account)**：安全机制说明、开票抬头与结构化账单地址表单维护。

### 2. 管理运维中心 (Admin Web)
- **健康监控看板 (Dashboard)**：实时汇聚 8 项核心运维指标、严重告警、24小时失败操作、Provider 健康状态、物理节点健康度及容量超过 85% 的预警面板。
- **结构化运维巡检与详情**：
  - **实例巡检**：主机硬件规格、网卡与 IP 分配、流量统计、关联操作任务流及折叠式诊断 JSON。
  - **操作任务管理**：阶段状态、百分比进度条、错误码排查、步骤明细表格（`admin.operation.steps`）、重试尝试历史（`admin.operation.attempts`）、重试与取消调度控制。
  - **Provider 监控**：驱动类型、通信版本、健康检查历史及延迟统计、关联节点分配。
  - **商品与套餐生命周期**：商品状态切换（草稿 `draft` / 已上架 `active` / 归档 `archived` 通过 `PATCH /api/v1/admin/products/{id}`）、套餐配置与新建模态框。
  - **审计追踪 (Audit)**：记录操作者邮箱、动作类型、目标资源、请求 ID 及变更前后（Before / After）数据差异对比。
  - **系统设置**：列表行直接唤起修改抽屉，支持敏感配置安全掩码。
  - **事务与死信**：死信队列异常事件监控与一键重放 (`POST /api/v1/admin/dead-letters/{id}/replay`)。
  - **财务退款**：原路退款弹窗与幂等防重机制。
  - **权限与安全**：细粒度 RBAC 角色勾选维护与管理员 TOTP 双重验证启用流程。

### 3. 底层架构与可靠性保障
- **领域真相源独立**：严禁合并 Order、Payment、Invoice、Subscription、Instance，各领域模型维护独立状态机。
- **双分录复式记账 (Double-Entry Ledger)**：账本记录只增不改，改动必记交易号，确保钱包资金链路可审计对账。
- **事务性发件箱 (Transactional Outbox)**：领域事件与数据库持久化位于同事务，Worker 异步投递，保证事件 At-Least-Once 可靠分发。
- **Provider 契约层 (Provider Contract)**：Runman Agent、LXD API 与 Mock 统一实现能力协商、状态映射、幂等执行与 Contract Tests。
- **异步长任务与恢复**：耗时运维任务一律采用 202 响应返回 Operation ID；Reconciler 负责漂移对齐，Worker 崩溃与 Redis 重启可无缝恢复。

---

## 使用预构建镜像部署 (V2)

镜像由 GitHub Actions 构建并发布到 GitHub Container Registry，支持 `linux/amd64` 与 `linux/arm64` 双架构：

- `ghcr.io/snail46/vps-billing-codex/backend:v2`
- `ghcr.io/snail46/vps-billing-codex/user-web:v2`
- `ghcr.io/snail46/vps-billing-codex/admin-web:v2`
- `ghcr.io/snail46/vps-billing-codex/gateway:v2`

### 1. 快速启动

复制部署模板并配置独立密钥（所有生产密钥须不少于 32 字符）：

```sh
cp deploy/.env.images.example .env
docker compose --env-file .env -f deploy/docker-compose.images.yml pull
docker compose --env-file .env -f deploy/docker-compose.images.yml up --detach --wait
```

如需部署特定版本标签：

```sh
VPS_BILLING_IMAGE_TAG=v2.0.0 docker compose --env-file .env -f deploy/docker-compose.images.yml up --detach --wait
```

### 2. 访问端点

- **用户前端**：`http://localhost:9090/`
- **管理前端**：`http://localhost:9092/`
- **存活探针 (Liveness)**：`http://localhost:9090/health/live`
- **就绪探针 (Readiness)**：`http://localhost:9090/health/ready`

> **网络隔离说明**：网关在 Gateway (Nginx) 层实现了用户端与管理端的端口级别隔离。用户端端口（9090）绝不暴露管理 API，管理端端口（9092）绝不暴露用户 API。

### 3. 生产安全与反向代理

当使用外部 Nginx、Caddy 或 Cloudflare Tunnel 终止公网 TLS 时，请修改 `.env`：

```dotenv
ALLOW_INSECURE_HTTP=false
COOKIE_SECURE=true
USER_WEB_ORIGIN=https://portal.example.com
ADMIN_WEB_ORIGIN=https://admin.example.com
```

将用户域名反代至 `http://127.0.0.1:9090`，管理域名反代至 `http://127.0.0.1:9092`。
请确保在反向代理中针对 `/api/v1/events` 开启长连接支持（如 Nginx 配置 `proxy_buffering off;`），以保障 SSE 运维推送顺畅。

---

## 从源码启动

```sh
docker compose --env-file .env -f deploy/docker-compose.yml up --build --detach --wait
```

---

## 初始化超级管理员

在 `.env` 中指定以下变量，Stack 启动后将在数据库迁移完成后自动创建 `super_admin`：

```dotenv
ADMIN_BOOTSTRAP_EMAIL=admin@example.com
ADMIN_BOOTSTRAP_DISPLAY_NAME=Administrator
ADMIN_BOOTSTRAP_PASSWORD=replace-with-a-strong-initial-admin-password
```

初始化具备幂等性：若该管理员已存在，不会重复覆写密码或产生冗余 Audit 日志。初始化完成后建议从生产环境中清除该变量。

---

## Provider 配置

- **LXD**：类型指定为 `lxdapi`；通过 `credential_ref` 配置 `<REF>_CLIENT_CERT_PEM`、`<REF>_CLIENT_KEY_PEM` 与 `<REF>_SERVER_CA_PEM`。
- **Runman**：将证书放入 `RUNMAN_TLS_DIRECTORY`，通过 `--profile runman` 启动 Gateway；在后台创建 Node 后执行 `/app/issue-agent-token --node-id <uuid>` 生成节点通信凭证。
- **Mock**：仅用于集成测试与本地验证，严禁用于生产环境。

---

## 质量验证

项目在 CI 与本地执行完整的自动化门禁检查：

- **Frontend**：
  - `npm run typecheck`：TypeScript 严格模式类型检查（Project References）。
  - `npm run lint`：ESLint 严格检查（`--max-warnings=0`）。
  - `npm test`：Vitest 自动化测试套件（包含双语词条全等校验与表单渲染）。
  - `npm run build`：生产环境打包验证。
- **Backend**：
  - `go vet ./...`：静态语法与类型诊断。
  - `go test ./...`：核心领域模型、状态机、Outbox、Provider Contract 与 Reconciler 单元/集成测试。
  - `go build ./cmd/...`：服务主程序及后台 Worker 二进制构建验证。
