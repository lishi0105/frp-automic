"""参数解析与主流程。"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from frps_deploy import config
from frps_deploy.certs import (
    docker_compose_up_all, docker_compose_up_initial,
    docker_compose_restart_nginx, issue_certs,
)
from frps_deploy.clean import clean_all, stop_all
from frps_deploy.config import ConfigFileCreated, load_runtime_config
from frps_deploy.console import (
    print, eprint, prompt_input,
    COLOR_BLUE, COLOR_GREEN, COLOR_RED, COLOR_YELLOW,
)
from frps_deploy.constants import BASE_DIR
from frps_deploy.docker_env import (
    check_docker_compose, check_docker_permission, check_frps_server_port,
    check_required_ports,
    get_latest_frp_version, get_public_ip, get_iface_by_ip, get_default_route_iface,
    verify_domains_resolve_to_ip,
)
from frps_deploy.generator import (
    ensure_dirs, generate_frpc_compose, generate_frpc_toml,
    generate_frps_compose, generate_frps_toml,
    generate_http_challenge_conf, generate_https_nginx_confs,
    remove_https_confs, write_generated_info,
)
from frps_deploy.models import DeployContext
from frps_deploy.output import print_generate_only_result, print_proxy_only_result, print_result
from frps_deploy.services import (
    all_remote_ports, exposed_http_remote_ports, force_all_services_tcp,
    managed_domains, tcp_remote_ports, validate_services,
)
from frps_deploy.state import (
    classify_services, load_deploy_state, previous_http_safe_aliases, write_deploy_state,
)
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
        "-p", "--proxy",
        action="store_true",
        help="只更新/启动代理相关配置，不自动申请证书；所有服务强制按 TCP 代理处理",
    )
    parser.add_argument(
        "-s", "--stop",
        action="store_true",
        help="停止所有已启动的 Docker Compose 服务，不删除文件或镜像",
    )
    parser.add_argument(
        "--clean",
        action="store_true",
        help="清理生成的容器、本地镜像和文件夹（慎用！）",
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


def _previous_int(previous: dict, key: str) -> int:
    try:
        return int(previous.get(key) or 0)
    except (TypeError, ValueError):
        return 0


def _previous_str(previous: dict, key: str) -> str:
    return str(previous.get(key) or "").strip()


def build_context(root_domain: str, email: str, previous: dict | None = None) -> DeployContext:
    previous = previous or {}
    can_reuse = _previous_str(previous, "root_domain") == root_domain
    generated_ports: set = set(all_remote_ports())
    if config.FRPS_SERVER_PORT >= 1000:
        if config.FRPS_SERVER_PORT in generated_ports:
            raise ValueError(f"frps.server_port 与 services port 冲突：{config.FRPS_SERVER_PORT}")
        bind_port = config.FRPS_SERVER_PORT
        generated_ports.add(bind_port)
    elif can_reuse and _previous_int(previous, "bind_port") >= 1000:
        bind_port = _previous_int(previous, "bind_port")
        if bind_port in generated_ports:
            raise ValueError(f"上次部署的 frps.server_port 与 services port 冲突：{bind_port}")
        generated_ports.add(bind_port)
    else:
        bind_port = random_free_port_excluding(generated_ports)

    previous_dashboard_port = _previous_int(previous, "dashboard_port") if can_reuse else 0
    if previous_dashboard_port >= 1000 and previous_dashboard_port not in generated_ports:
        dashboard_port = previous_dashboard_port
        generated_ports.add(dashboard_port)
    else:
        dashboard_port = random_free_port_excluding(generated_ports)
    if not config.STATUS_APP_ENABLED:
        status_port = 0
    elif config.STATUS_APP_PORT >= 1000:
        if config.STATUS_APP_PORT in generated_ports:
            raise ValueError(f"status.port 与已使用端口冲突：{config.STATUS_APP_PORT}")
        status_port = config.STATUS_APP_PORT
        generated_ports.add(status_port)
    elif can_reuse and _previous_int(previous, "status_port") >= 1000 and _previous_int(previous, "status_port") not in generated_ports:
        status_port = _previous_int(previous, "status_port")
        generated_ports.add(status_port)
    else:
        status_port = random_free_port_excluding(generated_ports)
    if config.VPS_PUBLIC_IP:
        vps_public_ip = validate_ipv4(config.VPS_PUBLIC_IP)
    elif can_reuse and _previous_str(previous, "vps_public_ip"):
        vps_public_ip = validate_ipv4(_previous_str(previous, "vps_public_ip"))
    else:
        vps_public_ip = validate_ipv4(get_public_ip())
    verify_domains_resolve_to_ip(managed_domains(root_domain), vps_public_ip)
    try:
        host_iface = get_default_route_iface()
    except Exception:
        host_iface = get_iface_by_ip(vps_public_ip)
    print(f"检测到公网IP对应网卡：{host_iface}")

    return DeployContext(
        root_domain=root_domain,
        email=email,
        vps_public_ip=vps_public_ip,
        host_iface=host_iface,
        frp_version=_previous_str(previous, "frp_version") if can_reuse and _previous_str(previous, "frp_version") else get_latest_frp_version(),
        bind_port=bind_port,
        dashboard_port=dashboard_port,
        status_port=status_port,
        token=config.FRPS_TOKEN or (_previous_str(previous, "token") if can_reuse else "") or random_password(32),
        dashboard_user=_previous_str(previous, "dashboard_user") if can_reuse and _previous_str(previous, "dashboard_user") else "admin",
        dashboard_password=_previous_str(previous, "dashboard_password") if can_reuse and _previous_str(previous, "dashboard_password") else random_password(16),
        status_user=config.STATUS_APP_USER,
        status_password=config.STATUS_APP_PASSWORD,
        suffix=_previous_str(previous, "suffix") if can_reuse and _previous_str(previous, "suffix") else random_letters(16),
    )


def generate_files(
    ctx: DeployContext,
    include_challenge: bool = True,
    previous_state: dict | None = None,
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
    if include_challenge:
        print("\n生成临时 HTTP challenge 配置...")
        remove_https_confs(previous_http_safe_aliases(previous_state or {}))
        generate_http_challenge_conf(ctx)


def print_service_plan(previous_state: dict) -> None:
    if not previous_state:
        print("\n未检测到历史部署状态，本次按首次部署处理。")
        return
    plan = classify_services(previous_state)
    print("\n服务配置差异：")
    print(f"  已部署且未变化，本次略过：{', '.join(plan['kept']) if plan['kept'] else '无'}", color=COLOR_GREEN)
    print(f"  已部署但配置变化，本次更新：{', '.join(plan['changed']) if plan['changed'] else '无'}", color=COLOR_YELLOW)
    print(f"  配置新增，本次部署：{', '.join(plan['added']) if plan['added'] else '无'}", color=COLOR_BLUE)
    print(f"  配置已删除，本次卸载入口：{', '.join(plan['removed']) if plan['removed'] else '无'}", color=COLOR_RED)


def prompt_firewall_confirmation(ctx: DeployContext) -> None:
    ports = [
        ("80/tcp",  "Nginx HTTP"),
        ("443/tcp", "Nginx HTTPS"),
        (f"{ctx.bind_port}/tcp", "frps 客户端连接"),
    ]
    for p in tcp_remote_ports():
        ports.append((f"{p}/tcp", "TCP 直通服务"))
    for p in exposed_http_remote_ports():
        ports.append((f"{p}/tcp", "HTTP 服务直通"))
    if config.FRPS_DASHBOARD_HTTP:
        ports.append((f"{ctx.dashboard_port}/tcp", "frps dashboard 公网访问"))
    if config.STATUS_APP_ENABLED and config.STATUS_APP_HTTP:
        ports.append((f"{ctx.status_port}/tcp", "状态页公网访问"))

    print("\n以下端口需要在服务器防火墙/云安全组中放行：")
    for port, desc in ports:
        print(f"  {port:<16} # {desc}", color=COLOR_RED)

    answer = prompt_input("\n确认已放行以上端口？继续部署请输入 y：")
    if answer.strip().lower() != "y":
        eprint("已取消部署。")
        sys.exit(0)


def apply_proxy_only(ctx: DeployContext, has_previous_state: bool = False) -> None:
    check_docker_compose()
    check_docker_permission()
    if not has_previous_state:
        check_frps_server_port(ctx.bind_port)
    print("\n启动/更新 frps 代理服务...")
    run(["docker", "compose", "up", "-d", "--remove-orphans", "frps"], cwd=BASE_DIR)
    if config.STATUS_APP_ENABLED:
        run(["docker", "compose", "up", "-d", "--remove-orphans", "frps-status"], cwd=BASE_DIR)


def main() -> None:
    args = parse_args()

    if args.clean:
        clean_all()
        return

    if args.stop:
        stop_all()
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

    previous_state = load_deploy_state()
    print_service_plan(previous_state)

    root_domain, email = prompt_user(require_email=not args.proxy)
    try:
        ctx = build_context(root_domain, email, previous_state)
    except Exception as exc:
        eprint(f"配置错误：{exc}")
        sys.exit(1)

    reusable_previous_state = _previous_str(previous_state, "root_domain") == root_domain

    generate_files(ctx, include_challenge=not args.proxy, previous_state=previous_state)

    if args.proxy:
        apply_proxy_only(ctx, has_previous_state=reusable_previous_state)
        write_deploy_state(ctx)
        print_proxy_only_result(ctx)
        return

    if not args.run:
        print_generate_only_result(ctx)
        return

    check_docker_compose()
    check_docker_permission()
    if not reusable_previous_state:
        check_required_ports(ctx.bind_port)
    prompt_firewall_confirmation(ctx)

    if args.run and not reusable_previous_state:
        print("\n全新部署前先停止可能残留的旧 Compose 服务，防止重复部署。", color=COLOR_YELLOW)
        stop_all()
        
    print("\n启动 frps + nginx，用于证书 HTTP 验证...")
    docker_compose_up_initial()

    print("\n申请 HTTPS 证书...")
    issue_certs(ctx)

    print("\n生成正式 HTTPS Nginx 反代配置...")
    generate_https_nginx_confs(ctx)

    print("\n重启 Nginx 应用 HTTPS 配置...")
    docker_compose_restart_nginx()

    print("\n启动所有容器（含 certbot 续期、frps-status）...")
    docker_compose_up_all()

    write_deploy_state(ctx)
    print_result(ctx)
