"""打印部署结果与摘要信息。"""
from __future__ import annotations

from frps_deploy import config
from frps_deploy.console import print
from frps_deploy.constants import (
    BASE_DIR, COMPOSE_FILE, FRPC_BASE_DIR, FRPC_COMPOSE_FILE, FRPC_TOML_FILE,
    FRPS_TOML_FILE, GENERATED_INFO_FILE, NGINX_CONF_DIR,
    STATUS_APP_DIR, STATUS_APP_ENV_FILE,
)
from frps_deploy.models import DeployContext
from frps_deploy.services import (
    dashboard_domain, http_services, local_ip, local_port, remote_port,
    status_domain, tcp_services,
)
from frps_deploy.utils import toml_str


def print_frpc_config(ctx: DeployContext) -> None:
    print("\n================ frpc.toml 示例 ================")
    print(f"serverAddr = {toml_str(ctx.vps_public_ip)}")
    print(f"serverPort = {ctx.bind_port}")
    print("")
    print('auth.method = "token"')
    print(f"auth.token = {toml_str(ctx.token)}")
    print("")
    print("[transport.tls]")
    print("enable = true")
    for item in config.SERVICES:
        print("")
        print(f"# {item.get('comment', item['alias'])}")
        print("[[proxies]]")
        print(f"name = {toml_str(str(item['alias']))}")
        print('type = "tcp"')
        print(f"localIP = {toml_str(local_ip(item))}")
        print(f"localPort = {local_port(item)}")
        print(f"remotePort = {remote_port(item)}")
    print("================================================")


def print_result(ctx: DeployContext) -> None:
    print("\n================ 部署完成 ================")
    print(f"生成目录：{BASE_DIR}")
    print(f"配置记录：{GENERATED_INFO_FILE}")
    print(f"frpc 目录：{FRPC_BASE_DIR}")

    if http_services():
        print("\nHTTPS 反代访问地址：")
        for item in http_services():
            print(f"  https://{item['alias']}.{ctx.root_domain}    # {item.get('comment', '')}")

    if config.STATUS_APP_ENABLED:
        print("\n状态页：")
        if config.STATUS_APP_HTTP:
            print(f"  http://{ctx.vps_public_ip}:{ctx.status_port}  （已开放公网，注意防火墙）")
        else:
            print(f"  http://127.0.0.1:{ctx.status_port}  （仅本机）")
        print(f"  https://{status_domain(ctx.root_domain)}")

    if tcp_services():
        print("\nTCP 直通端口：")
        for item in tcp_services():
            print(f"  {item.get('comment', item['alias'])}: {ctx.vps_public_ip}:{remote_port(item)} -> {local_ip(item)}:{local_port(item)}")

    print("\nfrps 客户端连接信息：")
    print(f"  serverPort = {ctx.bind_port}")
    print(f"  auth.token = {ctx.token}")
    print("\nfrps dashboard：")
    if config.FRPS_DASHBOARD_HTTP:
        print(f"  http://{ctx.vps_public_ip}:{ctx.dashboard_port}  （已开放公网，注意防火墙）")
    else:
        print(f"  仅 VPS 本机访问：http://127.0.0.1:{ctx.dashboard_port}")
    print(f"  https://{dashboard_domain(ctx.root_domain)}")
    print(f"  user     = {ctx.dashboard_user}")
    print(f"  password = {ctx.dashboard_password}")
    print("\n常用命令：")
    print(f"  cd {BASE_DIR}")
    print("  docker compose ps")
    print("  docker compose logs -f frps")
    print("  docker compose logs -f nginx")
    print("  docker compose logs -f certbot")
    print("  docker compose restart nginx")
    print(f"  cd {FRPC_BASE_DIR}")
    print("  docker compose up -d")
    if config.STATUS_APP_ENABLED:
        print(f"  cd {STATUS_APP_DIR}")
        print("  docker compose build && docker compose up -d")
    print_frpc_config(ctx)
    print("\n注意：")
    print("1. http 模式服务只在 Docker 内部 expose 给 nginx，外网不能直接用 IP:端口访问。")
    print("2. tcp 模式服务会直接把端口开放到公网，例如 GitLab SSH 的 5022。")
    print("3. frpc 和内网服务不在同一台机器时，请把 localIP 改成真实内网 IP。")
    print("4. 证书续期由 certbot 容器每 12 小时检查一次。")
    print("==========================================")


def print_generate_only_result(ctx: DeployContext) -> None:
    print("\n================ 文件生成完成 ================")
    print(f"生成目录：{BASE_DIR}")
    print(f"配置记录：{GENERATED_INFO_FILE}")
    print(f"frps 配置：{FRPS_TOML_FILE}")
    print(f"Docker Compose：{COMPOSE_FILE}")
    print(f"Nginx 配置目录：{NGINX_CONF_DIR}")
    if config.STATUS_APP_ENABLED:
        print(f"状态服务工程：{STATUS_APP_DIR}")
        print(f"状态服务环境：{STATUS_APP_ENV_FILE}")
        if config.STATUS_APP_HTTP:
            print(f"状态页 HTTP：http://{ctx.vps_public_ip}:{ctx.status_port}")
        else:
            print(f"状态页本机 HTTP：http://127.0.0.1:{ctx.status_port}")
    print(f"frpc 配置：{FRPC_TOML_FILE}")
    print(f"frpc Docker Compose：{FRPC_COMPOSE_FILE}")
    print("\n本次未执行启动、证书申请。")
    print("frps dashboard 部署后访问：")
    print(f"  https://{dashboard_domain(ctx.root_domain)}")
    if config.FRPS_DASHBOARD_HTTP:
        print(f"  http://{ctx.vps_public_ip}:{ctx.dashboard_port}")
    else:
        print(f"  http://127.0.0.1:{ctx.dashboard_port}")
    if config.STATUS_APP_ENABLED:
        print("状态页部署后访问：")
        print(f"  https://{status_domain(ctx.root_domain)}")
        if config.STATUS_APP_HTTP:
            print(f"  http://{ctx.vps_public_ip}:{ctx.status_port}")
        else:
            print(f"  http://127.0.0.1:{ctx.status_port}")
    print(f"需要继续执行部署启动时，请重新运行：")
    print(f"  vps-install-frps.py -c {config.CONFIG_FILE} -r")
    print_frpc_config(ctx)
