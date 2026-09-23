# VPS Billing Platform — AI Agent Starter Pack

这是 **VPS 财务计费与自动化资源管理平台 V1** 的工程施工包。

## 你怎么交给代码 Agent

### 最推荐
把整个目录/仓库交给 Agent，然后只发：

> 请严格按照 `AGENTS.md` 和 `MASTER_PROMPT.md` 施工。先阅读全部 P0 文档与当前任务相关文档，从 `TASKS.md` 第一个未完成任务开始。不要跨 Phase，不要猜第三方 API。每完成一个阶段必须运行 lint/test/build，并汇报修改、测试结果和遗留风险。

### 如果 Agent 只能接收少量文件
优先给：
1. `AGENTS.md`
2. `MASTER_PROMPT.md`
3. `TASKS.md`
4. `docs/01-PRD.md`
5. `docs/02-ARCHITECTURE.md`
6. `docs/04-DATABASE-SCHEMA.md`
7. `docs/05-STATE-MACHINES.md`
8. `docs/06-PROVIDER-CONTRACT.md`
9. `docs/07-WORKFLOW-SPEC.md`
10. `docs/08-API-CONTRACT.md`
11. `docs/10-USER-UX.md`
12. `docs/11-ADMIN-UX.md`
13. `docs/20-AI-IMPLEMENTATION-RULES.md`

## 总原则
- Business PostgreSQL 是唯一商业数据真相源。
- Provider 只负责实际基础设施状态与执行。
- 所有长操作必须进入 Operation/Workflow。
- 所有状态必须可见、可解释、可恢复。
- Provider 可替换，上层业务和体验不变。
- 内部代码/API/DB 全英文；UI 支持 `zh-CN` 与 `en-US`。
- User Web 站在用户视角；Admin Web 站在运营/运维视角。
- V1 模块化单体，不做无必要的微服务、Kafka、Kubernetes。

## Phase 0 本地启动

安装 Docker Compose 后，在仓库根目录执行：

```sh
docker compose -f deploy/docker-compose.yml up --build
```

用户端位于 `http://localhost:8080/`，管理端位于
`http://localhost:8080/admin/`，健康检查位于
`http://localhost:8080/health/live` 和 `/health/ready`。

首次管理员需显式创建；请先替换开发密钥，再在 server 容器中执行：

```sh
docker compose -f deploy/docker-compose.yml exec \
  -e ADMIN_BOOTSTRAP_PASSWORD='replace-with-a-strong-secret' \
  server /app/bootstrap-admin --email admin@example.com --display-name Administrator
```

## 首批 Provider 方向
Direct Provider：
- LXDAPI: https://github.com/xkatld/lxdapi-web-server
- CLICD: https://cli.cd/

Agent Provider：
- Runman Agent: https://github.com/narwhal-cloud/runman-agent

实现 Adapter 前必须核对第三方当前官方文档/源码；平台 Contract 不因第三方变化而改变。
