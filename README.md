# VPS Billing V1

VPS Billing V1 是一个面向 VPS 销售、计费和自动化交付的模块化单体平台。后端使用 Go、PostgreSQL、Redis；用户端与管理端使用 React + TypeScript，并支持 `zh-CN` 和 `en-US`。

## V1 能力

- 独立的 Order、Payment、Invoice、Subscription、Instance 聚合
- 双分录 Ledger、Wallet projection、幂等支付回调和事务 Outbox
- Operation + Workflow + Worker 长任务，以及 SSE/轮询恢复
- Mock、Direct LXD 和 Runman Agent Provider
- 用户购买、支付、自动开通、实例管理、续费、工单和通知
- 管理员 RBAC、Audit、TOTP 2FA、财务调整、节点和任务诊断
- Reconciler、Worker crash/Redis restart 恢复、节点离线处理
- Prometheus Metrics、安全响应头、加密备份/恢复和发布验证

## 使用预构建镜像部署

镜像由 GitHub Actions 构建并发布到 GitHub Container Registry，每个标签均提供 `linux/amd64` 与 `linux/arm64` manifest：

- `ghcr.io/snail46/vps-billing-codex/backend:v1`
- `ghcr.io/snail46/vps-billing-codex/user-web:v1`
- `ghcr.io/snail46/vps-billing-codex/admin-web:v1`

复制并修改镜像部署配置，所有生产密钥必须独立且不少于 32 字符：

```sh
cp deploy/.env.images.example .env
docker compose --env-file .env -f deploy/docker-compose.images.yml pull
docker compose --env-file .env -f deploy/docker-compose.images.yml up --detach --wait
```

使用固定版本部署时设置：

```sh
VPS_BILLING_IMAGE_TAG=v1.0.0 docker compose --env-file .env -f deploy/docker-compose.images.yml up --detach --wait
```

如果 GHCR Package 为 private，先执行：

```sh
echo "$GITHUB_TOKEN" | docker login ghcr.io -u USERNAME --password-stdin
```

访问地址：

- 用户端：`http://localhost:9090/`
- 管理端：`http://localhost:9090/admin/`
- Liveness：`http://localhost:9090/health/live`
- Readiness：`http://localhost:9090/health/ready`

默认镜像模板允许内网 HTTP，适用于无公网 IP 的设备。请把 `USER_WEB_ORIGIN` 和 `ADMIN_WEB_ORIGIN` 设置为浏览器实际访问的完整 Origin（含协议和非默认端口）。应用本身不终止公网 TLS：使用 Nginx、Caddy 等外部 HTTPS 反代或 Cloudflare Tunnel 时，应改为：

```dotenv
ALLOW_INSECURE_HTTP=false
COOKIE_SECURE=true
USER_WEB_ORIGIN=https://portal.example.com
ADMIN_WEB_ORIGIN=https://portal.example.com
```

此时外部入口使用 HTTPS，Tunnel/反代到本机 `http://localhost:9090` 即可。

## 从源码启动

```sh
docker compose --env-file .env -f deploy/docker-compose.yml up --build --detach --wait
```

## 初始化管理员

```sh
docker compose --env-file .env -f deploy/docker-compose.images.yml exec \
  -e ADMIN_BOOTSTRAP_PASSWORD='replace-with-a-strong-secret' \
  server /app/bootstrap-admin \
  --email admin@example.com \
  --display-name Administrator
```

## Provider 配置

- LXD：Provider 类型使用 `lxdapi`；通过 `credential_ref` 引用 `<REF>_CLIENT_CERT_PEM`、`<REF>_CLIENT_KEY_PEM` 和 `<REF>_SERVER_CA_PEM`。
- Runman：将 `tls.crt`、`tls.key` 放入 `RUNMAN_TLS_DIRECTORY`，使用 `--profile runman` 启动 Gateway；创建 Provider 与 Node 后执行 `/app/issue-agent-token --node-id <uuid>`。
- Mock Provider 仅用于开发和验收，禁止用于生产。

## 发布与运维

- 发布流程与回滚：[docs/21-RELEASE-RUNBOOK.md](docs/21-RELEASE-RUNBOOK.md)
- 部署规范：[docs/17-DEPLOYMENT.md](docs/17-DEPLOYMENT.md)
- OpenAPI：[docs/openapi/openapi.yaml](docs/openapi/openapi.yaml)
- 数据库结构：[db/schema.sql](db/schema.sql)

加密备份、受控恢复和发布验证脚本位于 `deploy/scripts/`。内部 `/metrics` 要求 `METRICS_TOKEN`，默认不会经公网 Reverse Proxy 暴露。

## 验证

GitHub Actions 会执行 Go fmt/vet/race tests/build、npm audit/lint/test/typecheck/build、完整 Compose E2E、100 次重复支付回调，以及加密备份恢复演练。
