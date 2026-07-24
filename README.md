# FlareDNS

[![CI](https://github.com/mikusaa/flaredns/actions/workflows/ci.yml/badge.svg)](https://github.com/mikusaa/flaredns/actions/workflows/ci.yml)
[![Container](https://github.com/mikusaa/flaredns/actions/workflows/publish-image.yml/badge.svg)](https://github.com/mikusaa/flaredns/actions/workflows/publish-image.yml)
[![Release](https://img.shields.io/github/v/release/mikusaa/flaredns)](https://github.com/mikusaa/flaredns/releases)
[![License](https://img.shields.io/github/license/mikusaa/flaredns)](LICENSE)

FlareDNS 是一个轻量、自托管的 Cloudflare DNS 管理面板，适用于 Homelab、个人服务器和多 VPS 环境。它提供专注于 DNS 的日常管理界面，不包含 CDN、WAF 或 Zero Trust 等 Cloudflare 平台功能。

当前版本采用单管理员模型，支持多个 Cloudflare API Token、多个 Zone，以及密码和 Passkey 登录。

## 功能

- 多 Cloudflare API Token 与多 Zone 管理
- A、AAAA、CNAME、TXT、MX、SRV、CAA 记录的创建、编辑与删除
- 记录搜索、类型与代理筛选、分页和批量操作
- Cloudflare Proxy 开关，启用代理时自动设置 Auto TTL
- 密码登录及多个 Passkey，支持无用户名 Passkey 登录
- 操作日志与脱敏字段差异
- AES-256-GCM Token 加密与 SQLite 持久化
- 响应式界面及亮色、暗色主题
- 单容器 Docker Compose 部署

Cloudflare 返回的其他记录类型以只读方式显示。

## 部署

运行环境需要 Docker Engine 24+ 和 Docker Compose v2。

### 使用预构建镜像

```bash
git clone https://github.com/mikusaa/flaredns.git
cd flaredns
cp .env.example .env
```

将 `.env` 中的镜像设置为：

```dotenv
FLAREDNS_IMAGE=ghcr.io/mikusaa/flaredns:latest
```

启动服务：

```bash
docker compose pull
docker compose up -d --no-build
```

### 从源码构建

```bash
git clone https://github.com/mikusaa/flaredns.git
cd flaredns
cp .env.example .env
docker compose up -d --build
```

### 首次登录

首次启动会创建 `admin` 账号，随机密码仅在账号创建时写入一次容器日志：

```bash
docker compose logs flaredns | grep 'initial administrator created'
```

管理界面默认位于 `http://localhost:8080`。登录后可在“API Token”页面连接 Cloudflare 账号。

服务状态检查：

```bash
docker compose ps
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

宿主机端口变更时，`FLAREDNS_PORT` 与 `FLAREDNS_PUBLIC_URL` 应保持一致：

```dotenv
FLAREDNS_PORT=8081
FLAREDNS_PUBLIC_URL=http://localhost:8081
```

## Cloudflare API Token

推荐为 FlareDNS 创建专用 API Token，并将授权范围限制在需要管理的 Zone。所需最小权限：

- `Zone / DNS / Edit`
- `Zone / Zone / Read`

FlareDNS 不接收 Cloudflare 主账号密码，也不在 API 或管理界面中回显 Token。通过验证的 Token 使用 `/data/master.key` 加密后存入 SQLite。

## Passkey 与 HTTPS

Passkey 可在“设置 -> Passkey”中注册和管理。密码登录始终保留，用于首次配置与账号恢复。

- `http://localhost` 属于浏览器认可的安全上下文，可用于本机测试。
- 通过局域网 IP 的普通 HTTP 地址访问时，仅密码登录可用。
- 域名部署需要 HTTPS，且 `FLAREDNS_PUBLIC_URL` 必须与浏览器地址一致。
- RP ID 默认取自 `FLAREDNS_PUBLIC_URL` 的主机名，也可通过 `FLAREDNS_RP_ID` 固定。

反向代理配置示例：

```dotenv
FLAREDNS_PUBLIC_URL=https://dns.example.com
FLAREDNS_RP_ID=dns.example.com
FLAREDNS_TRUSTED_PROXIES=172.16.0.0/12
```

反向代理需要保留 `Host`、`X-Forwarded-For` 和 `X-Forwarded-Proto`。`FLAREDNS_TRUSTED_PROXIES` 仅应包含实际代理的 IP 或 CIDR。

> 修改 RP ID 后，原 RP ID 下注册的 Passkey 将无法使用。迁移期间应保留密码入口，并在新地址下重新注册 Passkey。

## 配置

Docker Compose 自动读取项目根目录的 `.env`。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `FLAREDNS_IMAGE` | `flaredns:local` | 容器镜像，可设置为 GHCR 镜像 |
| `FLAREDNS_PORT` | `8080` | 宿主机映射端口 |
| `FLAREDNS_ADDR` | `:8080` | 容器内监听地址 |
| `FLAREDNS_PUBLIC_URL` | `http://localhost:8080` | 外部访问地址及 WebAuthn Origin |
| `FLAREDNS_RP_ID` | Public URL 主机名 | WebAuthn Relying Party ID |
| `FLAREDNS_SESSION_TTL` | `12h` | Session 有效期，最小 `5m` |
| `FLAREDNS_TRUSTED_PROXIES` | 空 | 可信代理 CIDR，多个值用逗号分隔 |
| `FLAREDNS_LOG_LEVEL` | `info` | 日志级别：`info` 或 `debug` |
| `FLAREDNS_COOKIE_SECURE` | 由 Public URL 推导 | Cookie Secure 属性覆盖值 |
| `TZ` | `Asia/Shanghai` | 容器时区 |

## 管理员密码恢复

服务运行时可生成新的随机管理员密码：

```bash
docker compose exec flaredns flaredns reset-password
```

重置操作会解除登录锁定并撤销现有 Session，不会删除 Cloudflare Token、Zone、DNS 配置或 Passkey。新密码不会写入操作日志，登录后可在“设置”中修改。

## 数据与备份

所有持久化数据位于 `./data`：

| 文件 | 用途 |
| --- | --- |
| `flaredns.db`、`flaredns.db-wal` | SQLite 数据库及 WAL |
| `master.key` | Cloudflare Token 的 AES-256-GCM 主密钥 |

数据库与主密钥必须成组备份和恢复。`master.key` 丢失后，数据库中的 Token 无法解密；当数据库含有 Token 而主密钥缺失时，FlareDNS 会拒绝启动。

创建一致性备份：

```bash
docker compose stop flaredns
tar -czf "flaredns-backup-$(date +%Y%m%d%H%M%S).tar.gz" data
docker compose start flaredns
```

恢复时应先停止服务，再完整恢复备份中的 `data` 目录。数据库与 `master.key` 不应分开恢复。

## 升级

升级前应先备份 `data` 目录。

使用 GHCR 镜像部署：

```bash
docker compose pull
docker compose up -d --no-build
```

从源码构建：

```bash
git pull --ff-only
docker compose up -d --build
```

服务启动时会在事务中自动执行数据库迁移。升级后可通过以下命令确认状态：

```bash
docker compose ps
docker compose logs --tail=100 flaredns
```

## 架构

生产镜像采用多阶段构建。Vue 静态资源嵌入 Go 二进制，最终由单个 Alpine 容器提供 Web UI 与 API。

```text
flaredns/
├── frontend/          Vue 3 + TypeScript + Vite + Naive UI
├── backend/           Go + Gin + SQLite + WebAuthn
├── data/              运行数据，仅提交 .gitkeep
├── Dockerfile
└── docker-compose.yml
```

DNS 记录实时从 Cloudflare 读取，不复制到 SQLite。本地数据库保存 Zone 缓存、设置、认证数据和审计日志。

## 本地开发

开发环境需要 Go 1.25、Node.js 22 和 npm。

```bash
cd frontend
npm install
cd ..

# 分别在两个终端运行
make dev-backend
make dev-frontend
```

前端开发服务器运行在 `http://localhost:5173`，并将 `/api` 请求代理到 `http://localhost:8080`。

测试与构建：

```bash
make test
make build
docker build -t flaredns:local .
```

## 镜像发布

GitHub Actions 自动构建 `linux/amd64` 和 `linux/arm64` 镜像并推送至 `ghcr.io/mikusaa/flaredns`：

- `main` 分支发布 `main`、`latest` 和 `sha-<commit>` 标签。
- `v*` Git 标签发布语义化版本标签。例如 `v1.2.3` 对应 `1.2.3`、`1.2` 和 `1`。
- `Publish container image` 工作流支持手动触发。

工作流使用仓库内置的 `GITHUB_TOKEN` 写入 GitHub Packages，无需单独配置 Registry 凭据。

## 安全设计

- 密码使用 Argon2id 哈希，连续失败会触发账号锁定。
- Session 使用服务端不透明随机令牌，Cookie 设置 HttpOnly 与 SameSite=Strict。
- 非只读请求同时校验 CSRF Token 与 Origin。
- WebAuthn 校验 Challenge、Origin、RP ID、用户验证与签名计数。
- WebAuthn Challenge 单次使用并在过期后清理。
- Cloudflare Token 使用独立随机 nonce 的 AES-256-GCM 加密。
- 密码、Session、Challenge、Token 及完整 WebAuthn 响应不会进入审计日志。

公网部署应启用 HTTPS、限制管理入口来源，并保护宿主机和 `data` 备份。

## 项目范围

- 单管理员，不包含多用户权限模型。
- 不包含 DDNS、ACME DNS Challenge、Docker Label 自动发现或 Terraform Provider。
- 操作日志长期保留，当前不提供自动清理策略。
- 本项目与 Cloudflare, Inc. 无隶属或官方关系。

## License

FlareDNS 基于 [MIT License](LICENSE) 发布，允许使用、修改、分发及商业使用，但必须保留版权与许可证声明。
