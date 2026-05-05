"""参数解析与主流程。"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from frps_deploy import config
from frps_deploy.certs import (
    docker_compose_up_all, docker_compose_up_initial,
    docker_compose_up_status_app, docker_compose_restart_nginx,
    issue_certs,
)
from frps_deploy.clean import clean_all
from frps_deploy.config import ConfigFileCreated, load_runtime_config
from frps_deploy.console import print, eprint, prompt_input
from frps_deploy.constants import BASE_DIR, STATUS_APP_DIR
from frps_deploy.docker_env import (
    check_docker_compose, check_docker_permission, check_frps_server_port,
    check_required_ports,
    get_latest_frp_version, get_public_ip,
)
from frps_deploy.generator import (
    ensure_dirs, generate_frpc_compose, generate_frpc_toml,
    generate_frps_compose, generate_frps_toml,
    generate_http_challenge_conf, generate_https_nginx_confs,
    remove_https_confs, write_generated_info, write_status_app_env,
)
from frps_deploy.models import DeployContext
from frps_deploy.output import print_generate_only_result, print_proxy_only_result, print_result
from frps_deploy.services import all_remote_ports, force_all_services_tcp, validate_services
from frps_deploy.utils import (
    random_free_port_excluding, random_letters, random_password, run,
    validate_ipv4,
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="FRPS + Nginx + Certbot 自动部署脚本")
    parser.add_argument(
        "-c", "--config",
        default=None,
        help="JSON 配置文件路径，默认读取当前目录的 frps-config.json",
    )
    parser.add_argument(
        "-r", "--run",
        action="store_true",
        help="生成相关文件后继续执行证书申请和容器启动；默认只生成文件",
    )
    parser.add_argument(
        "-b", "--build",
        action="store_true",
        help="只编译 frps-status-app 的 Docker 镜像，不执行其他部署步骤",
    )
    parser.add_argument(
        "-p", "--proxy",
        action="store_true",
        help="只更新/启动代理相关配置，不自动申请证书；所有服务强制按 TCP 代理处理",
    )
    parser.add_argument(
        "--clean",
        action="store_true",
        help="清理生成的容器、本地镜像和文件夹（先通过 docker 修正权限再删除）",
    )
    return parser.parse_args()


def set_config_file(path_text: str | None) -> None:
    path = Path(path_text).expanduser() if path_text else Path.cwd() / "frps-config.json"
    if not path.is_absolute():
        path = Path.cwd() / path
    config.CONFIG_FILE = path.resolve()


def prompt_user(require_email: bool = True) -> tuple[str, str]:
    root_domain = config.ROOT_DOMAIN
    if root_domain:
        print(f"使用配置文件中的主域名：{root_domain}")
    else:
        root_domain = prompt_input("请输入主域名，例如 hello.com：").strip().lower()
    if not root_domain or "." not in root_domain:
        eprint("域名格式不正确。")
        sys.exit(1)

    email = config.CERT_EMAIL
    if not require_email:
        if email:
            print(f"使用配置文件中的证书邮箱：{email}")
        return root_domain, email

    if email:
        print(f"使用配置文件中的证书邮箱：{email}")
    else:
        email = prompt_input("请输入证书邮箱：").strip()
    if "@" not in email:
        eprint("邮箱格式不正确。")
        sys.exit(1)

    return root_domain, email


def build_context(root_domain: str, email: str) -> DeployContext:
    generated_ports: set = set(all_remote_ports())
    if config.FRPS_SERVER_PORT >= 1000:
        if config.FRPS_SERVER_PORT in generated_ports:
            raise ValueError(f"frps.server_port 与 services port 冲突：{config.FRPS_SERVER_PORT}")
        bind_port = config.FRPS_SERVER_PORT
        generated_ports.add(bind_port)
    else:
        bind_port = random_free_port_excluding(generated_ports)

    dashboard_port = random_free_port_excluding(generated_ports)
    if not config.STATUS_APP_ENABLED:
        status_port = 0
    elif config.STATUS_APP_PORT >= 1000:
        if config.STATUS_APP_PORT in generated_ports:
            raise ValueError(f"status.port 与已使用端口冲突：{config.STATUS_APP_PORT}")
        status_port = config.STATUS_APP_PORT
        generated_ports.add(status_port)
    else:
        status_port = random_free_port_excluding(generated_ports)
    vps_public_ip = validate_ipv4(config.VPS_PUBLIC_IP) if config.VPS_PUBLIC_IP else validate_ipv4(get_public_ip())

    return DeployContext(
        root_domain=root_domain,
        email=email,
        vps_public_ip=vps_public_ip,
        frp_version=get_latest_frp_version(),
        bind_port=bind_port,
        dashboard_port=dashboard_port,
        status_port=status_port,
        token=config.FRPS_TOKEN or random_password(32),
        dashboard_user="admin",
        dashboard_password=random_password(16),
        suffix=random_letters(16),
    )


def generate_files(
    ctx: DeployContext,
    include_challenge: bool = True,
) -> None:
    ensure_dirs()
    print("\n生成 frps.toml...")
    generate_frps_toml(ctx)
    print("生成 frps/docker-compose.yml...")
    generate_frps_compose(ctx)
    print("生成 frpc/frpc.toml...")
    generate_frpc_toml(ctx)
    print("生成 frpc/docker-compose.yml...")
    generate_frpc_compose(ctx)
    print("写入部署信息记录...")
    write_generated_info(ctx)
    if config.STATUS_APP_ENABLED:
        print("写入 frps-status-app/.env...")
        write_status_app_env(ctx)
    if include_challenge:
        print("\n生成临时 HTTP challenge 配置...")
        remove_https_confs()
        generate_http_challenge_conf(ctx)


def apply_proxy_only(ctx: DeployContext) -> None:
    check_docker_compose()
    check_docker_permission()
    check_frps_server_port(ctx.bind_port)
    print("\n启动/更新 frps 代理服务...")
    run(["docker", "compose", "up", "-d", "frps"], cwd=BASE_DIR)
    if config.STATUS_APP_ENABLED:
        print("\n编译并启动状态服务工程...")
        docker_compose_up_status_app()


def build_status_app() -> None:
    print("=== 编译 frps-status-app Docker 镜像 ===")
    check_docker_compose()
    check_docker_permission()
    if not STATUS_APP_DIR.exists():
        eprint(f"错误：未找到 frps-status-app 目录：{STATUS_APP_DIR}")
        sys.exit(1)
    run(["docker", "compose", "build"], cwd=STATUS_APP_DIR)
    print(f"\n编译完成。启动容器请执行：")
    print(f"  cd {STATUS_APP_DIR}")
    print("  docker compose up -d")


def main() -> None:
    args = parse_args()

    if args.clean:
        clean_all()
        return

    if args.build:
        build_status_app()
        return

    print("=== FRPS + Nginx + Certbot 自动部署脚本 ===")
    set_config_file(args.config)
    print(f"使用配置文件：{config.CONFIG_FILE}")

    try:
        load_runtime_config()
        if args.proxy:
            force_all_services_tcp()
        validate_services()
    except ConfigFileCreated:
        print("已只生成默认配置文件，本次不执行部署。")
        sys.exit(0)
    except Exception as exc:
        eprint(f"配置错误：{exc}")
        sys.exit(1)

    root_domain, email = prompt_user(require_email=not args.proxy)
    try:
        ctx = build_context(root_domain, email)
    except Exception as exc:
        eprint(f"配置错误：{exc}")
        sys.exit(1)
    generate_files(ctx, include_challenge=not args.proxy)

    if args.proxy:
        apply_proxy_only(ctx)
        print_proxy_only_result(ctx)
        return

    if not args.run:
        print_generate_only_result(ctx)
        return

    check_docker_compose()
    check_docker_permission()
    check_required_ports(ctx.bind_port)

    print("\n启动 frps + nginx，用于证书 HTTP 验证...")
    docker_compose_up_initial()

    print("\n申请 HTTPS 证书...")
    issue_certs(ctx)

    print("\n生成正式 HTTPS Nginx 反代配置...")
    generate_https_nginx_confs(ctx)

    if config.STATUS_APP_ENABLED:
        print("\n编译并启动状态服务工程...")
        docker_compose_up_status_app()

    print("\n重启 Nginx 应用 HTTPS 配置...")
    docker_compose_restart_nginx()

    print("\n启动 certbot 自动续期容器...")
    docker_compose_up_all()

    print_result(ctx)
