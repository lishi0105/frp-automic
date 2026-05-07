"""Docker 环境检查、frp 版本获取。"""
from __future__ import annotations

import json
import socket
import subprocess
import sys
import urllib.request
from typing import Iterable

from frps_deploy.console import print, eprint
from frps_deploy.constants import DEFAULT_FRP_VERSION
from frps_deploy.utils import command_exists, is_port_free


def check_docker_compose() -> None:
    if not command_exists("docker"):
        eprint("错误：未找到 docker 命令，请先安装 Docker。")
        sys.exit(1)
    ret = subprocess.run(
        ["docker", "compose", "version"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    if ret.returncode != 0:
        eprint("错误：当前 Docker 不支持 docker compose 插件。")
        eprint("请先安装 docker-compose-plugin。")
        eprint(ret.stderr.strip())
        sys.exit(1)
    print(ret.stdout.strip())


def check_docker_permission() -> None:
    ret = subprocess.run(
        ["docker", "info"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    if ret.returncode != 0:
        eprint("错误：当前用户没有 Docker 权限，或者 Docker 未启动。")
        eprint("可尝试：")
        eprint("  sudo systemctl enable --now docker")
        eprint("  sudo usermod -aG docker $USER")
        eprint("然后重新登录 SSH。")
        eprint(ret.stderr.strip())
        sys.exit(1)
    print("Docker 权限检查通过。")


def check_frps_server_port(port: int) -> None:
    if not is_port_free(port, host="127.0.0.1"):
        eprint(f"错误：frps server_port {port}/tcp 已被本机占用，Docker 将无法绑定该端口。")
        eprint("请更换 frps.server_port，或停止占用该端口的服务后重试。")
        sys.exit(1)


def check_required_ports(frps_server_port: int) -> None:
    check_frps_server_port(frps_server_port)
    for p in (80, 443):
        if not is_port_free(p, host="127.0.0.1"):
            eprint(f"警告：本机端口 {p} 可能已被占用，后续 docker compose 可能启动失败。")
            eprint("如果你已经有宝塔/Nginx/Caddy 占用 80/443，需要先停掉，或改成接入现有反代。")


def get_latest_frp_version() -> str:
    url = "https://api.github.com/repos/fatedier/frp/releases/latest"
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "frp-stack-deploy-script"})
        with urllib.request.urlopen(req, timeout=10) as resp:
            data = json.loads(resp.read().decode("utf-8"))
            tag = data.get("tag_name")
            if isinstance(tag, str) and tag.startswith("v"):
                print(f"检测到 frp 最新版本：{tag}")
                return tag
    except Exception as exc:
        print(f"获取 frp 最新版本失败，使用默认版本 {DEFAULT_FRP_VERSION}，原因：{exc}")
    return DEFAULT_FRP_VERSION


def get_public_ip() -> str:
    urls = [
        "https://api.ipify.org",
        "https://ifconfig.me/ip",
        "https://ipv4.icanhazip.com",
    ]
    for url in urls:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "frp-stack-deploy-script"})
            with urllib.request.urlopen(req, timeout=8) as resp:
                ip = resp.read().decode("utf-8", errors="replace").strip()
            if ip:
                print(f"检测到 VPS 公网 IP：{ip}")
                return ip
        except Exception:
            continue
    raise RuntimeError("无法自动获取 VPS 公网 IP，请在 frps-config.json 中填写 vps_public_ip")


def resolve_domain_ipv4s(domain: str) -> list[str]:
    try:
        infos = socket.getaddrinfo(domain, None, family=socket.AF_INET, type=socket.SOCK_STREAM)
    except socket.gaierror as exc:
        eprint(f"解析域名 {domain} 失败，可能是 DNS 问题或域名不存在。")
        raise RuntimeError(f"域名无法解析：{domain}，原因：{exc}") from exc

    ips = sorted({addr for *_, sockaddr in infos for addr in [sockaddr[0]] if isinstance(addr, str)})
    if not ips:
        eprint(f"域名 {domain} 没有解析到 IPv4 地址。")
        raise RuntimeError(f"域名未解析到 IPv4 A 记录：{domain}")
    return ips


def verify_domains_resolve_to_ip(domains: Iterable[str], expected_ip: str) -> None:
    checked: set[str] = set()
    for domain in domains:
        domain = domain.strip().rstrip(".").lower()
        if not domain or domain in checked:
            continue
        checked.add(domain)

        ips = resolve_domain_ipv4s(domain)
        if expected_ip not in ips:
            raise RuntimeError(
                f"域名解析 IP 不匹配：{domain} 当前解析到 {', '.join(ips)}，"
                f"但检测到的 VPS 公网 IP 是 {expected_ip}。请修改域名 A 记录后重试。"
            )
        print(f"域名解析检查通过：{domain} -> {expected_ip}")


def get_default_route_iface() -> str:
    """Resolve default outbound interface via `ip route get 1.1.1.1`."""
    ret = subprocess.run(
        ["ip", "route", "get", "1.1.1.1"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if ret.returncode != 0:
        raise RuntimeError(f"无法通过路由识别默认网卡：{ret.stderr.strip()}")
    parts = ret.stdout.strip().split()
    if "dev" in parts:
        idx = parts.index("dev")
        if idx + 1 < len(parts):
            iface = parts[idx + 1].strip()
            if iface and iface != "lo":
                return iface
    raise RuntimeError("默认路由未返回有效网卡（dev）")


def get_iface_by_ip(ip: str) -> str:
    """
    Resolve host interface name by IPv4 address.
    Prefer `ip route get <ip>` output, fallback to `ip -o -4 addr show`.
    """
    # Keep this function as fallback only; prefer get_default_route_iface().

    ret = subprocess.run(
        ["ip", "-o", "-4", "addr", "show"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    if ret.returncode != 0:
        raise RuntimeError(f"无法识别公网IP对应网卡（ip -o -4 addr show 执行失败）：{ret.stderr.strip()}")
    for line in ret.stdout.splitlines():
        # Example: 2: eth0    inet 1.2.3.4/24 brd ...
        cols = line.split()
        if len(cols) >= 4 and cols[2] == "inet":
            iface = cols[1]
            addr = cols[3].split("/")[0]
            if addr == ip:
                return iface
    raise RuntimeError(f"无法识别公网IP对应网卡：{ip}")
