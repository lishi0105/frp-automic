# 内网穿透运维工具

基于 frp 的内网穿透场景，提供从服务端部署、域名入口配置、HTTPS 证书申领到运行状态监控、流量统计和异常告警的一体化运维能力。它用于快速搭建可公网访问的内网服务入口，并持续观察代理、证书和主机网络状态，减少手动配置、巡检和告警处理成本。

工具集包含两个组件：

- **`vps-install-frps.py`** — 一键部署脚本，自动生成 frps / Nginx / Certbot / frpc 配置，申请域名证书并启动容器
- **`frps-status-app/`** — 基于 Vue 3 + Go 的 Web 监控面板，实时展示代理状态、流量趋势、证书有效期，并支持 SMTP 告警

---
## 许可证

本项目采用 Apache License 2.0 许可证。

详情请查看 [LICENSE](./LICENSE) 文件。

## 一、快速开始

### 1.1. 前置条件

- 域名必须已自行配置通配符 A 记录（`*.<root_domain>`），并解析到部署该服务的云服务器公网 IPv4 地址。
- 云服务器需要开放80/443端口以及至少一个 TCP 端口供 frps 使用（脚本会自动输出需要开放的端口并提示）。

### 1.2. 配置

生成并编辑配置文件：

```bash
python3 vps-install-frps.py
```

未指定 `-c/--config` 时，脚本读取当前目录的 `frps-config.json`；如果该文件不存在会生成默认配置文件并退出，效果等同于 `-c frps-config.json`。需要使用其他路径时通过 `-c/--config` 指定；指定路径不存在时同样会生成默认配置文件并退出。

`frps-config.json` 主要字段：

| 字段                  | 说明                                                         |
| --------------------- | ------------------------------------------------------------ |
| `root_domain`         | 根域名，如 `example.com`                                     |
| `vps_public_ip`       | VPS 公网 IP；留空时自动获取，并写入生成的 `frpc/frpc.toml`   |
| `cert_email`          | Certbot 注册邮箱                                             |
| `frps.server_port`    | frps serverPort；小于 `1000` 或留空时随机生成                |
| `frps.token`          | frps 认证 token；留空时随机生成                              |
| `frps.dashboard_http` | 是否允许 frps dashboard 通过 `VPS_IP:dashboardPort` 公网直通，默认关闭 |
| `frps.enable_prometheus` | 是否在 frps dashboard `/metrics` 开启 Prometheus 指标，默认开启 |
| `frpc.use_encryption` | 是否为生成的 frpc 代理开启传输加密，默认开启                  |
| `frpc.use_compression` | 是否为生成的 frpc 代理开启传输压缩，默认开启                 |
| `status.enabled`      | 是否启用状态面板与 `status.<root_domain>` 反代，默认启用     |
| `status.port`         | 状态面板本机 HTTP 端口；小于 `1000` 或留空时随机生成         |
| `status.http`         | 是否允许状态面板通过 `VPS_IP:status.port` 公网直通，默认关闭 |
| `services`            | 服务列表，见下方说明                                         |

需要将所有 HTTP 服务域名，以及 `frps.<root_domain>`、`status.<root_domain>` 的 A 记录解析到 VPS 公网 IP。禁用 `status.enabled` 时不需要配置 `status.<root_domain>`。
启动 frps 前脚本会检查 `frps.server_port/tcp` 是否被本机占用，并检查常见本机防火墙是否放行；云服务器还需要在云厂商安全组中放行该 TCP 端口。
脚本会检查 JSON 配置文件结构、`services` 条目类型、必填端口、端口范围（`1000` 到 `65535`），以及 `services` 中的 `alias` 和 `port` 是否重复。

服务列表示例：

```jsonc
{
  "frps": {
    "server_port": 0,
    "token": "",
    "dashboard_http": false,
    "enable_prometheus": true
  },
  "frpc": {
    "use_encryption": true,
    "use_compression": true
  },
  "status": {
    "enabled": true,
    "port": 0,
    "http": false
  },
  "services": [
    {
      "alias": "emby",
      "comment": "Emby 媒体服务",
      "mode": "http",      // http = Nginx HTTPS 反代；tcp = 直接暴露端口
      "tunnel": true,      // 是否通过 frp 穿透，缺省 true；false 表示服务已在 VPS 本机
      "port": 8096,        // frps 侧端口（frpc remotePort）
      "local_port": 8096,  // 内网真实端口（缺省等于 port）
      "local_ip": "127.0.0.1", // 内网真实 IP（缺省为127.0.0.1)
      "expose_http_port": false // http 模式下是否额外开放 VPS_IP:port，默认 false
    },
    {
      "alias": "alist",
      "comment": "VPS 本机 Alist",
      "mode": "http",
      "tunnel": false,
      "port": 5244,
      "local_port": 5244,
      "local_ip": "127.0.0.1"
    }
  ]
}
```

`tunnel: false` 适用于服务已经运行在 VPS 本机的场景。HTTP 服务仍会生成 `https://<alias>.<root_domain>` 的 Nginx 反代和证书配置，但不会生成 frpc 代理；Nginx 会直接转发到 VPS 本机的 `local_ip:local_port`。

### 1.3. 部署 frps

```bash
# 仅生成配置文件，不启动
python3 vps-install-frps.py

# 生成配置 + 申请证书 + 启动所有容器（等价：--run）
python3 vps-install-frps.py -r

# 只更新/启动代理相关配置，不自动申请证书；所有服务强制按 TCP 代理处理
python3 vps-install-frps.py -p

# 清理脚本生成的容器、镜像和目录
python3 vps-install-frps.py --clean

# 停止所有已启动的 Docker Compose 服务，不删除文件或镜像
python3 vps-install-frps.py -s

# 指定配置文件
python3 vps-install-frps.py -c /path/to/frps-config.json -r
```

运行 `-r/--run` 时，脚本会读取上次部署记录（`frps/.deploy-state.json`，旧版本则回退读取 `frps/.env.generated.txt`），并与当前 JSON 配置中的 `services` 对比：

- 已部署且配置未变化的服务会保留，不重新生成随机端口、token 或容器后缀；
- 配置新增的服务会加入 frps/frpc/Nginx/证书管理；
- 配置已删除的服务会从 frps/frpc/Nginx 入口配置中移除；
- alias 相同但端口、模式、穿透方式等发生变化的服务会按新配置更新。

已存在的证书不会重复申请，后续续期仍由 certbot 容器处理。已删除服务的 Nginx 入口会被移除，但证书目录不会自动删除。

脚本会在当前目录生成：

```
frps/
  docker-compose.yml
  frps.toml
  .deploy-state.json
  nginx/conf.d/
  certbot/
frpc/
  docker-compose.yml
  frpc.toml
```

将 `frpc/` 目录复制到需要做内网穿透的机器，运行 `docker compose up -d` 即可连接。

### 1.4. 访问方式

HTTP 模式默认只通过 Nginx 域名入口访问：

```text
https://<alias>.<root_domain>
```

如果某个 HTTP 服务配置了 `"expose_http_port": true`，脚本会额外开放调试入口：

```text
<VPS_IP>:<services[].port>
```

使用 `-p/--proxy` 时不会申请证书，也不会改写现有 Nginx HTTPS 配置；此时所有服务都会忽略配置文件中的 `mode` 和 `expose_http_port`，强制按 TCP 代理处理并开放 `VPS_IP:port` 直连入口。

TCP 模式服务始终开放：

```text
<VPS_IP>:<services[].port>
```

状态面板启用时，部署完成后会输出两类地址：

```text
http://127.0.0.1:<status.port>
https://status.<root_domain>
```

frps dashboard 始终生成 HTTPS 反代：

```text
https://frps.<root_domain>
```

`frps.dashboard_http: false` 时 dashboard 端口只绑定 `127.0.0.1`，不能通过 `VPS_IP:dashboardPort` 直接访问。`status.http: false` 时状态面板 HTTP 端口只绑定 `127.0.0.1`，公网通过 `status.<root_domain>` 的 HTTPS 反代访问。

`status.enabled: false` 时不会生成状态面板反代，也不会启动 `frps-status-app`。

---

## 二、frps-status-app — 监控面板

默认用户名密码：admin/admin123
登录入口：status.<root_domain>

### 2.1. 功能

| 页面     | 内容                                                         |
| -------- | ------------------------------------------------------------ |
| 数据看板 | frps 在线状态、本月流量与网卡流量汇总、证书预警、代理统计、本月趋势图 |
| 代理列表 | 代理在线状态、连接数、月度流量、证书关联，后端分页/排序/筛选 |
| 证书列表 | 证书有效期、剩余天数、公网 TLS 握手状态、关联代理与异常信息  |
| 历史统计 | 每日流量明细、本月 Top 5、趋势折线图、导出 CSV               |
| 流量统计 | 公网 IP/网卡维度日流量汇总，支持按日期查询入站、出站与总量趋势 |
| 系统配置 | SMTP 告警（含测试发送）、流量阈值、数据库压缩与清理          |

### 2.2. 环境变量

复制示例文件（可按默认配置）：

```bash
cd frps-status-app
cp .env.example .env
```

| 变量                      | 默认值                           | 说明                                                    |
| ------------------------- | -------------------------------- | ------------------------------------------------------- |
| `LISTEN`                  | `0.0.0.0:8080`                   | 容器内监听地址                                          |
| `STATUS_APP_BIND`         | `127.0.0.1`                      | 映射到宿主机的绑定地址；设为 `0.0.0.0` 才允许公网直通   |
| `STATUS_APP_PORT`         | `28080`                          | 映射到宿主机的 HTTP 端口                                |
| `DB_PATH`                 | `/data/frps-status.sqlite`       | SQLite 数据库路径                                       |
| `FRPS_HOST`               | `frps`                           | frps 所在主机                                           |
| `FRPS_BIND_PORT`          | `7000`                           | frps 服务端口                                           |
| `FRPS_DASHBOARD_PORT`     | `7500`                           | frps Dashboard 端口                                     |
| `FRPS_DASHBOARD_USER`     | —                                | Dashboard 用户名                                        |
| `FRPS_DASHBOARD_PASSWORD` | —                                | Dashboard 密码                                          |
| `STATUS_DOMAINS`          | —                                | 逗号分隔的域名，用于证书检测                            |
| `STATUS_USER`             | —                                | 面板登录用户名（留空不鉴权）                            |
| `STATUS_PASSWORD`         | —                                | 面板登录密码                                            |
| `CERT_DIR`                | `/etc/letsencrypt/live`          | 证书目录                                                |
| `POLL_SECONDS`            | `60`                             | 数据轮询间隔（秒）                                      |
| `HOST_PUBLIC_IP`          | —                                | 宿主机公网 IPv4（用于网卡流量统计；部署脚本会自动写入） |
| `HOST_IFACE`              | —                                | 公网 IP 对应网卡名（如 `eth0`，部署脚本会自动写入）     |
| `HOST_NET_STATS_DIR`      | `/host-net-stats`                | 容器内网卡统计目录                                      |
| `HOST_NET_STATS_MOUNT`    | `/sys/class/net/eth0/statistics` | 宿主机映射路径（手动部署时需按实际网卡调整）            |

### 2.3. Docker 部署（推荐）

通常由 `vps-install-frps.py --run` 自动写入 `.env` 并启动。状态服务默认加入 `frps_default` Docker 网络，供 Nginx 通过容器内网反代；单独部署前需确保 frps 侧 compose 已启动并创建该网络。
为采集宿主机网卡日流量，部署脚本会自动识别公网 IP 对应网卡并将其统计目录只读挂载到容器（`/host-net-stats`）。

单独部署时：

```bash
cd frps-status-app
cp .env.example .env   # 编辑 .env
docker compose up -d --build
```

### 2.4. 本地开发

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

### 2.5. API 接口

| 方法     | 路径                       | 说明                                                         |
| -------- | -------------------------- | ------------------------------------------------------------ |
| POST     | `/api/login`               | 面板登录                                                     |
| POST     | `/api/logout`              | 退出登录                                                     |
| GET      | `/api/session`             | 会话状态                                                     |
| GET      | `/api/status`              | 完整快照（frps、代理、证书、流量）                           |
| GET      | `/api/proxies`             | 代理分页列表（`page/page_size/sort/order/keyword/type/online`） |
| GET      | `/api/certificates`        | 证书分页列表（`page/page_size/sort/order/keyword/status/tls`） |
| GET      | `/api/daily`               | 每日流量明细                                                 |
| GET      | `/api/daily/interface`     | 网卡/公网IP维度日流量（`from/to`）                           |
| GET      | `/api/daily/export`        | 导出 CSV                                                     |
| GET/POST | `/api/settings`            | 读取 / 保存配置                                              |
| POST     | `/api/settings/test-email` | 发送测试邮件                                                 |
| POST     | `/api/db/vacuum`           | SQLite VACUUM                                                |
| POST     | `/api/db/purge`            | 清理旧数据 `{"days": 60}`                                    |

除 `/api/login`、`/api/session`、`/api/user/forgot-password` 外，其余接口需要登录态（会话 Cookie）。

---

## 三、目录结构

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
│   │       ├── views/        # Dashboard / ProxyList / CertificateList / Statistics / TrafficStatistics / Settings / Login
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

## 四、依赖

- Docker Engine + docker compose 插件
- Go 1.22+（本地开发）
- Node.js 20+（前端开发）
