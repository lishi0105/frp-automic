# frp-automic

FRP 服务端一体化部署与监控工具集。包含两个组件：

- **`vps-install-frps.py`** — 一键部署脚本，自动生成 frps / Nginx / Certbot / frpc 配置并启动容器
- **`frps-status-app/`** — 基于 Vue 3 + Go 的 Web 监控面板，实时展示代理状态、流量趋势和证书有效期

---

## 快速开始

### 1. 配置

复制并编辑配置文件：

```bash
cp frps-config.json.example frps-config.json   # 若不存在，首次运行脚本会自动生成
```

`frps-config.json` 主要字段：

| 字段 | 说明 |
|------|------|
| `root_domain` | 根域名，如 `example.com` |
| `vps_public_ip` | VPS 公网 IP（留空则自动获取） |
| `cert_email` | Certbot 注册邮箱 |
| `frps_server_port` | frps serverPort；小于 `1000` 或留空时随机生成 |
| `frps_token` | frps 认证 token；留空时随机生成 |
| `status_app_enabled` | 是否启用状态面板与 `/_frps-status/` 反代，默认启用 |
| `status_app_port` | 状态面板本机监听端口；小于 `1000` 或留空时随机生成 |
| `dns_provider` | DNS 解析方式：`manual` 或 `cloudflare` |
| `cf_api_token` | Cloudflare API Token（仅 cloudflare 模式需要） |
| `services` | 服务列表，见下方说明 |

服务列表示例：

```jsonc
{
  "services": [
    {
      "alias": "emby",
      "comment": "Emby 媒体服务",
      "mode": "http",      // http = Nginx HTTPS 反代；tcp = 直接暴露端口
      "port": 7096,        // frps 侧端口（frpc remotePort）
      "local_port": 8096,  // 内网真实端口（缺省等于 port）
      "local_ip": "127.0.0.1",
      "expose_http_port": false // http 模式下是否额外开放 VPS_IP:port，默认 false
    }
  ]
}
```

### 2. 部署 frps

```bash
# 仅生成配置文件，不启动
python3 vps-install-frps.py

# 生成配置 + 申请证书 + 启动所有容器（等价：--run）
python3 vps-install-frps.py -r

# 仅编译 frps-status-app 镜像（等价：--build）
python3 vps-install-frps.py -b

# 清理脚本生成的容器、镜像和目录
python3 vps-install-frps.py --clean

# 指定配置文件
python3 vps-install-frps.py -c /path/to/frps-config.json -r
```

脚本会在当前目录生成：

```
frps/
  docker-compose.yml
  frps.toml
  nginx/conf.d/
  certbot/
frpc/
  docker-compose.yml
  frpc.toml
frps-status-app/.env
frps-status-app/data/
```

将 `frpc/` 目录复制到内网机器，运行 `docker compose up -d` 即可连接。

### 3. 访问方式

HTTP 模式默认只通过 Nginx 域名入口访问：

```text
https://<alias>.<root_domain>
```

如果某个 HTTP 服务配置了 `"expose_http_port": true`，脚本会额外开放调试入口：

```text
<VPS_IP>:<services[].port>
```

TCP 模式服务始终开放：

```text
<VPS_IP>:<services[].port>
```

状态面板启用时，部署完成后会输出两类地址：

```text
http://127.0.0.1:<status_app_port>
https://<alias>.<root_domain>/_frps-status/
```

`status_app_enabled: false` 时不会生成 Nginx 状态面板反代，也不会启动 `frps-status-app`。

---

## frps-status-app — 监控面板

### 功能

| 页面 | 内容 |
|------|------|
| 数据看板 | frps 在线状态、本月流量进度条、证书预警、代理统计、30 天趋势图 |
| 代理列表 | 代理在线状态、连接数、月度流量，支持搜索与过滤 |
| 历史统计 | 每日流量明细、本月 Top 5、趋势柱状图、导出 CSV |
| 系统配置 | SMTP 告警（含测试发送）、流量阈值、数据库压缩与清理 |

### 环境变量

复制示例文件：

```bash
cd frps-status-app
cp .env.example .env
```

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `LISTEN` | `127.0.0.1:28080` | 监听地址 |
| `DB_PATH` | `/data/frps-status.sqlite` | SQLite 数据库路径 |
| `FRPS_HOST` | `127.0.0.1` | frps 所在主机 |
| `FRPS_BIND_PORT` | `7000` | frps 服务端口 |
| `FRPS_DASHBOARD_PORT` | `7500` | frps Dashboard 端口 |
| `FRPS_DASHBOARD_USER` | — | Dashboard 用户名 |
| `FRPS_DASHBOARD_PASSWORD` | — | Dashboard 密码 |
| `STATUS_DOMAINS` | — | 逗号分隔的域名，用于证书检测 |
| `STATUS_USER` | — | 面板登录用户名（留空不鉴权） |
| `STATUS_PASSWORD` | — | 面板登录密码 |
| `CERT_DIR` | `/etc/letsencrypt/live` | 证书目录 |
| `POLL_SECONDS` | `60` | 数据轮询间隔（秒） |

### Docker 部署（推荐）

通常由 `vps-install-frps.py --run` 自动写入 `.env` 并启动。单独部署时：

```bash
cd frps-status-app
cp .env.example .env   # 编辑 .env
docker compose up -d --build
```

### 本地开发

**后端：**

```bash
cd frps-status-app
go run main.go
```

**前端（热重载）：**

```bash
cd frps-status-app/frontend
npm install
npm run dev   # 代理 /api 至 127.0.0.1:28080
```

**构建前端资源：**

```bash
cd frps-status-app/frontend
npm run build  # 输出至 ../web/
```

### API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/status` | 完整快照（frps、代理、证书、流量） |
| GET | `/api/daily` | 每日流量明细 |
| GET | `/api/daily/export` | 导出 CSV |
| GET/POST | `/api/settings` | 读取 / 保存配置 |
| POST | `/api/settings/test-email` | 发送测试邮件 |
| POST | `/api/db/vacuum` | SQLite VACUUM |
| POST | `/api/db/purge` | 清理旧数据 `{"days": 30}` |

所有接口均使用 HTTP Basic Auth（与面板登录凭据相同）。

---

## 目录结构

```
frp-automic/
├── frps-config.json          # 部署配置（含密钥，已加入 .gitignore）
├── vps-install-frps.py       # 一键部署脚本
├── frps-status-app/
│   ├── main.go               # 程序入口
│   ├── Dockerfile            # 多阶段构建（Node → Go → Alpine）
│   ├── docker-compose.yml
│   ├── .env.example
│   ├── frontend/             # Vue 3 + Vite 源码
│   │   └── src/
│   │       ├── views/        # Dashboard / ProxyList / Statistics / Settings
│   │       ├── api/          # API 封装
│   │       └── utils/        # 格式化工具
│   ├── src/                  # Go 包
│   │   ├── config/           # 配置加载
│   │   ├── model/            # 数据模型
│   │   ├── store/            # SQLite 操作
│   │   ├── frps/             # FRPS API 对接
│   │   ├── mail/             # SMTP 发送
│   │   └── server/           # HTTP 路由与处理器
│   └── web/                  # 前端构建产物（由 CI/Docker 生成）
├── frps/                     # 由部署脚本生成
└── frpc/                     # 由部署脚本生成，复制到内网机器使用
```

## 依赖

- Docker Engine + docker compose 插件
- Go 1.22+（本地开发）
- Node.js 20+（前端开发）
