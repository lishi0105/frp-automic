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
    http_domains, http_services, local_ip, local_port, remote_port, tcp_services,
)
from frps_deploy.utils import toml_str


def print_dns_notice(ctx: DeployContext) -> None:
    if not http_services():
        return
    print("\n请确保以下域名已经 A 记录解析到当前 VPS 公网 IP：")
    for domain in http_domains(ctx.root_domain):
        print(f"  {domain}  ->  {ctx.public_ip}")
    print("\n也可以直接配置泛解析：")
    print(f"  *.{ctx.root_domain}  ->  {ctx.public_ip}")


def print_frpc_config(ctx: DeployContext) -> None:
    print("\n================ frpc.toml 示例 ================")
    print(f'serverAddr = "{ctx.public_ip}"')
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
        print("\n状态页：")
        for item in http_services():
            print(f"  https://{item['alias']}.{ctx.root_domain}/_frps-status/")

    if tcp_services():
        print("\nTCP 直通端口：")
        for item in tcp_services():
            print(f"  {item.get('comment', item['alias'])}: {ctx.public_ip}:{remote_port(item)} -> {local_ip(item)}:{local_port(item)}")

    print("\nfrps 客户端连接信息：")
    print(f"  serverPort = {ctx.bind_port}")
    print(f"  auth.token = {ctx.token}")
    print("\nfrps dashboard：")
    print(f"  仅 VPS 本机访问：http://127.0.0.1:{ctx.dashboard_port}")
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
    print(f"  cd {STATUS_APP_DIR}")
    print("  docker compose build && docker compose up -d")
    print_frpc_config(ctx)
    print("\n注意：")
    print("1. http 模式服务只在 Docker 内部 expose 给 nginx，外网不能直接用 IP:端口访问。")
    print("2. tcp 模式服务会直接把端口开放到公网，例如 GitLab SSH 的 5022。")
    print("3. frpc 和内网服务不在同一台机器时，请把 localIP 改成真实内网 IP。")
    print("4. 证书续期由 certbot 容器每 12 小时检查一次。")
    print("5. Cloudflare 自动解析会把 A 记录设置为 DNS only，即 proxied=false。")
    print("==========================================")


def print_generate_only_result(ctx: DeployContext) -> None:
    print("\n================ 文件生成完成 ================")
    print(f"生成目录：{BASE_DIR}")
    print(f"配置记录：{GENERATED_INFO_FILE}")
    print(f"frps 配置：{FRPS_TOML_FILE}")
    print(f"Docker Compose：{COMPOSE_FILE}")
    print(f"Nginx 配置目录：{NGINX_CONF_DIR}")
    print(f"状态服务工程：{STATUS_APP_DIR}")
    print(f"状态服务环境：{STATUS_APP_ENV_FILE}")
    print(f"frpc 配置：{FRPC_TOML_FILE}")
    print(f"frpc Docker Compose：{FRPC_COMPOSE_FILE}")
    print("\n本次未执行启动、证书申请或 DNS 等待。")
    if http_services():
        print("状态页部署后访问：")
        for item in http_services():
            print(f"  https://{item['alias']}.{ctx.root_domain}/_frps-status/")
    print(f"需要继续执行部署启动时，请重新运行：")
    print(f"  vps-install-frps.py -c {config.CONFIG_FILE} -r")
    print_dns_notice(ctx)
    print_frpc_config(ctx)
