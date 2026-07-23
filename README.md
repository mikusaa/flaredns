# FlareDNS

FlareDNS 是一个轻量、自托管的 Cloudflare DNS 管理面板，面向 Homelab、个人服务器和多 VPS 场景。它专注于 DNS 管理，不尝试替代 Cloudflare 的 CDN、WAF 或 Zero Trust 控制台。

> 当前为 MVP：单管理员、多 Cloudflare API Token、密码与 Passkey 登录。

## 功能

- 多 Cloudflare API Token 与多 Zone 管理
- A、AAAA、CNAME、TXT、MX、SRV、CAA 记录的创建、编辑和删除
- 实时搜索、类型/代理筛选、分页和批量操作
- Cloudflare Proxy 开关；开启代理时自动使用 Auto TTL
- 密码登录与多个 Passkey；支持无需输入用户名的 Passkey 登录
- 操作日志与字段差异查看
- SQLite 持久化，Cloudflare Token 使用 AES-256-GCM 加密
- 响应式界面、亮色/暗色主题
- 单容器 Docker Compose 部署

Cloudflare 返回的其他 DNS 记录类型仍会显示，但 MVP 不允许编辑。

## 快速开始

要求：Docker Engine 24+ 与 Docker Compose v2。

克隆或下载本仓库后，在项目根目录执行：

```bash
cp .env.example .env
docker compose up -d --build
```

首次启动会创建 `admin` 用户，并且只在首次创建时输出一次随机密码：

```bash
docker compose logs flaredns | grep 'initial administrator created'
```

打开 `http://localhost:8080`，登录后前往“API Token”添加 Cloudflare API Token。

也可以直接使用 GitHub Container Registry 上的预构建镜像，无需本地构建。将 `.env` 中的镜像地址替换为仓库实际所有者：

```dotenv
FLAREDNS_IMAGE=ghcr.io/mikusaa/flaredns:latest
```

然后执行：

```bash
docker compose pull
docker compose up -d --no-build
```

查看运行状态：

```bash
docker compose ps
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

如果修改宿主机端口，`FLAREDNS_PORT` 和 `FLAREDNS_PUBLIC_URL` 必须保持一致。例如：

```dotenv
FLAREDNS_PORT=8081
FLAREDNS_PUBLIC_URL=http://localhost:8081
```

## Cloudflare Token

建议创建专用 API Token，并只授权需要管理的 Zone。最小权限：

- `Zone / DNS / Edit`
- `Zone / Zone / Read`

FlareDNS 不保存 Cloudflare 主账号密码，也不会在 API 或页面中回显 Token。Token 经验证后使用 `/data/master.key` 加密保存。

## Passkey 与 HTTPS

登录后前往“设置 -> Passkey”注册设备。密码入口始终保留，作为首次设置和恢复方式。

- `http://localhost` 是浏览器认可的安全上下文，可用于本机测试。
- 局域网 IP 的普通 HTTP 地址只能使用密码登录。
- 使用域名访问时必须配置 HTTPS，并将 `FLAREDNS_PUBLIC_URL` 设置为浏览器实际访问的完整地址。
- RP ID 默认从 `FLAREDNS_PUBLIC_URL` 的主机名推导，也可通过 `FLAREDNS_RP_ID` 显式设置。

示例：

```dotenv
FLAREDNS_PUBLIC_URL=https://dns.example.com
FLAREDNS_RP_ID=dns.example.com
FLAREDNS_TRUSTED_PROXIES=172.16.0.0/12
```

反向代理必须保留 `Host`、`X-Forwarded-For` 和 `X-Forwarded-Proto`。只将真实代理的 IP/CIDR 加入 `FLAREDNS_TRUSTED_PROXIES`。

不要在没有迁移计划时修改 RP ID。浏览器将无法使用旧 RP ID 下注册的 Passkey；修改前应保留密码入口，并在新地址下重新注册 Passkey。

## 配置

Compose 会自动读取项目根目录的 `.env`。

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `FLAREDNS_IMAGE` | `flaredns:local` | 容器镜像；可设置为 GHCR 发布的镜像 |
| `FLAREDNS_PORT` | `8080` | 宿主机映射端口 |
| `FLAREDNS_ADDR` | `:8080` | 容器内监听地址，通常无需修改 |
| `FLAREDNS_PUBLIC_URL` | `http://localhost:8080` | 外部访问地址、WebAuthn Origin |
| `FLAREDNS_RP_ID` | Public URL 主机名 | WebAuthn Relying Party ID |
| `FLAREDNS_SESSION_TTL` | `12h` | Session 有效期，最小 `5m` |
| `FLAREDNS_TRUSTED_PROXIES` | 空 | 可信代理 CIDR，多个值用逗号分隔 |
| `FLAREDNS_LOG_LEVEL` | `info` | `info` 或 `debug` |
| `FLAREDNS_COOKIE_SECURE` | 由 Public URL 推导 | 高级覆盖项，HTTPS 部署应为 `true` |
| `TZ` | `Asia/Shanghai` | 容器时区 |

## 管理员密码恢复

如果首次密码日志已丢失或忘记密码，可在服务运行时生成新的随机密码：

```bash
docker compose exec flaredns flaredns reset-password
```

该命令会解除登录锁定并撤销现有 Session，但不会删除 Cloudflare Token、Zone、DNS 配置或 Passkey。密码本身不会进入操作日志。登录后建议立即在“设置”中修改密码。

## 数据、备份与恢复

所有持久化数据位于 `./data`：

| 文件 | 用途 |
| --- | --- |
| `flaredns.db`、`flaredns.db-wal` | SQLite 数据库及 WAL |
| `master.key` | Cloudflare Token 的 AES-256-GCM 主密钥 |

数据库和主密钥必须作为一个整体备份。丢失 `master.key` 后，已有 Token 无法解密；FlareDNS 会拒绝在“数据库仍有 Token 但密钥缺失”的情况下启动。

为获得一致备份，先停止服务再归档数据目录：

```bash
docker compose stop flaredns
tar -czf "flaredns-backup-$(date +%Y%m%d%H%M%S).tar.gz" data
docker compose start flaredns
```

恢复时停止服务，将备份中的整个 `data` 目录恢复到项目根目录，再启动容器。不要只恢复数据库或只恢复主密钥。

## 升级

```bash
docker compose stop flaredns
tar -czf "flaredns-backup-$(date +%Y%m%d%H%M%S).tar.gz" data
git pull --ff-only
docker compose build --pull
docker compose up -d
```

启动时会在事务中自动执行数据库迁移。升级后检查：

```bash
docker compose ps
docker compose logs --tail=100 flaredns
```

## 架构

生产镜像由多阶段 Docker 构建生成：Vue 静态资源嵌入 Go 二进制，最终由一个 Alpine 容器提供 Web UI 与 API。

```text
flaredns/
├── frontend/          Vue 3 + TypeScript + Vite + Naive UI
├── backend/           Go + Gin + SQLite + WebAuthn
├── data/              运行数据，仅提交 .gitkeep
├── Dockerfile
└── docker-compose.yml
```

DNS 记录实时从 Cloudflare 获取，不复制到 SQLite；本地只缓存 Zone、记录数量、设置与审计日志。

## 本地开发

要求：Go 1.25、Node.js 22、npm。

```bash
cd frontend
npm install
cd ..

# 分别在两个终端运行
make dev-backend
make dev-frontend
```

前端开发服务器位于 `http://localhost:5173`，并将 `/api` 代理到 `http://localhost:8080`。

验证与构建：

```bash
make test
make build
docker build -t flaredns:local .
```

## 容器镜像发布

GitHub Actions 会在以下情况自动构建并推送 `linux/amd64`、`linux/arm64` 镜像到 `ghcr.io/mikusaa/flaredns`：

- 推送到默认的 `main` 分支：发布 `main`、`latest` 和 `sha-<commit>` 标签。
- 推送 `v*` Git 标签：例如 `v1.2.3` 会发布 `1.2.3`、`1.2`、`1` 和提交哈希标签。
- 在 Actions 页面手动运行 `Publish container image`。

工作流使用仓库内置的 `GITHUB_TOKEN`，无需添加 Registry 密钥。仓库或组织的 Actions 设置必须允许工作流写入 Packages。首次发布后，可在 GitHub Package 设置中将镜像可见性设为 Public。

## 安全设计

- 密码使用 Argon2id 哈希；连续失败会触发账户锁定。
- Session 为服务端不透明随机令牌，Cookie 使用 HttpOnly 与 SameSite=Strict。
- 非只读请求校验 CSRF Token 与 Origin。
- WebAuthn 校验 challenge、Origin、RP ID、用户验证和签名计数；challenge 单次使用并自动过期。
- Cloudflare Token 使用独立随机 nonce 的 AES-256-GCM 加密，页面和日志均不回显 Token。
- 密码、Session、Challenge、Token 和完整 WebAuthn 响应不会写入审计日志。

公网部署仍应使用 HTTPS、限制管理入口来源，并妥善保护宿主机与 `data` 备份。

## MVP 边界

- 仅支持单管理员，不包含多用户权限模型。
- 不包含 DDNS、ACME DNS Challenge、Docker Label 自动发现或 Terraform Provider。
- 操作日志在 MVP 中长期保留，不提供自动清理策略。
- 本项目与 Cloudflare, Inc. 无隶属或官方关系。

## License

FlareDNS 使用 [MIT License](LICENSE) 开源。你可以自由使用、修改、分发和用于商业项目，但需保留原版权与许可证声明。
