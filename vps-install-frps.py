#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
FRPS + Nginx + Certbot 一体化部署脚本

功能：
1. 使用 -r/--run 时检查 docker compose 是否可用；
2. 使用 -r/--run 时检查当前用户是否有 docker 权限；
3. 自动获取 fatedier/frp 最新版本，失败则使用默认版本；
4. 自动生成 frps bindPort、dashboard port、token、dashboard 密码；
5. 支持服务列表 JSON 配置：
   - mode = "http"：走 Nginx HTTPS 反代，生成 alias.root_domain 证书；
   - mode = "tcp" ：不反代，直接公网开放 remote port，例如 GitLab SSH；
6. 支持 local_port：
   - port       表示 VPS/frps 侧端口，也就是 frpc 的 remotePort；
   - local_port 表示内网真实服务端口，缺失时默认等于 port；
7. 支持 DNS 自动解析：
   - manual     ：手动解析；
   - cloudflare ：通过 Cloudflare API 自动创建/更新 A 记录；
8. 分别生成 frps/docker-compose.yml、frps/frps.toml、nginx 配置，以及 frpc/docker-compose.yml、frpc/frpc.toml；
9. 使用 certbot webroot 模式申请证书，并启动自动续期容器；
10. 容器名格式：服务名 + '_' + 16 位随机英文大小写后缀。

使用前提：
- 当前机器已经安装 Docker Engine 和 docker compose 插件；
- 当前用户可以执行 docker 命令；
- VPS 的 80/443 端口可公网访问；
- HTTP 模式服务的域名已经解析到当前 VPS 公网 IP；
- frpc 客户端需要使用脚本输出的 bindPort 和 token。

Cloudflare 自动解析用法：
- 推荐使用 API Token，不要使用 Global API Key；
- Token 至少需要：Zone:Read、DNS:Edit；
- 将 cf_api_token 填入 frps-config.json；
- 可选：将 vps_public_ip 填入 frps-config.json，留空则自动获取后确认。

配置文件：
- 默认读取脚本同目录 frps-config.json；
- 可用 -c/--config 指定配置文件路径；
- 配置文件不存在时只生成默认配置文件，然后退出。
- 默认只生成相关配置文件，不启动容器、不申请证书；加 -r/--run 才执行部署启动流程。
- 使用 -b/--build 时只编译 frps-status-app 的 Docker 镜像，不执行其他部署步骤。
- 使用 --clean 时停止并删除所有生成的容器（status-app 本地镜像一并删除），先通过 docker alpine
  容器将 frps/、frpc/、frps-status-app/data/ 的权限修改为 777，再删除这些目录以及 frps-status-app/.env。
"""

from frps_deploy.main import main

if __name__ == "__main__":
    main()
