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
"""

from __future__ import annotations

import builtins
import argparse
import ipaddress
import json
import os
import random
import secrets
import shutil
import socket
import string
import subprocess
import sys
import time
import urllib.parse
import urllib.request
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# ==========================================================
# 默认配置
#
# 字段说明：
#   alias      ：服务别名。http 模式下会作为子域名前缀，例如 emby.example.com；tcp 模式仅用于注释/代理名。
#   comment    ：说明文字。
#   mode       ：http 或 tcp。
#   port       ：VPS/frps 侧端口，也就是 frpc 的 remotePort。
#   local_port ：内网真实服务端口，也就是 frpc 的 localPort。可缺省，缺省时等于 port。
#   local_ip   ：内网服务 IP。可缺省，缺省为 127.0.0.1。
#
# 注意：
#   - http 模式适合 Emby、Web 管理页面、API 服务等 HTTP 服务；
#   - tcp 模式适合 GitLab SSH、SSH、RTSP、MQTT、数据库等非 HTTP 服务；
#   - tcp 模式会直接把 port 暴露到公网；
#   - DNS 自动解析只处理 http 模式服务，不处理 tcp 模式服务。
# ==========================================================
DEFAULT_SERVICES: List[Dict[str, Any]] = [
    {
        "alias": "emby",
        "comment": "Emby 媒体服务",
        "mode": "http",
        "port": 7096,
        "local_port": 8096,
        "local_ip": "127.0.0.1",
    },
    {
        "alias": "gitlab",
        "comment": "GitLab 服务",
        "mode": "http",
        "port": 5080,
        # local_port 缺省，表示 localPort = 5080
    },
    {
        "alias": "speedtest",
        "comment": "Speedtest 服务",
        "mode": "http",
        "port": 3010,
        # local_port 缺省，表示 localPort = 3010
    },
    {
        "alias": "gitlab_ssh",
        "comment": "GitLab SSH",
        "mode": "tcp",
        "port": 5022,
        "local_port": 22,
        "local_ip": "127.0.0.1",
    },
]

DEFAULT_FRP_VERSION = "v0.68.1"
SCRIPT_DIR = Path(__file__).resolve().parent
DEFAULT_CONFIG_FILE = SCRIPT_DIR / "frps-config.json"
CONFIG_FILE = DEFAULT_CONFIG_FILE
BASE_DIR = SCRIPT_DIR / "frps"
NGINX_CONF_DIR = BASE_DIR / "nginx" / "conf.d"
CERTBOT_CONF_DIR = BASE_DIR / "certbot" / "conf"
CERTBOT_WWW_DIR = BASE_DIR / "certbot" / "www"
STATUS_APP_DIR = SCRIPT_DIR / "frps-status-app"
STATUS_APP_ENV_FILE = STATUS_APP_DIR / ".env"
STATUS_APP_DATA_DIR = STATUS_APP_DIR / "data"
GENERATED_INFO_FILE = BASE_DIR / ".env.generated.txt"
COMPOSE_FILE = BASE_DIR / "docker-compose.yml"
FRPS_TOML_FILE = BASE_DIR / "frps.toml"
FRPC_BASE_DIR = SCRIPT_DIR / "frpc"
FRPC_COMPOSE_FILE = FRPC_BASE_DIR / "docker-compose.yml"
FRPC_TOML_FILE = FRPC_BASE_DIR / "frpc.toml"

DNS_PROVIDER_MANUAL = "manual"
DNS_PROVIDER_CLOUDFLARE = "cloudflare"
SUPPORTED_DNS_PROVIDERS = {DNS_PROVIDER_MANUAL, DNS_PROVIDER_CLOUDFLARE}

DEFAULT_CONFIG: Dict[str, Any] = {
    "root_domain": "",
    "cert_email": "",
    "dns_provider": "",
    "cf_api_token": "",
    "cf_zone_id": "",
    "vps_public_ip": "",
    "services": DEFAULT_SERVICES,
}

CONFIG: Dict[str, Any] = {}
SERVICES: List[Dict[str, Any]] = []
ROOT_DOMAIN = ""
CERT_EMAIL = ""
CF_API_TOKEN = ""
VPS_PUBLIC_IP = ""
CF_ZONE_ID = ""


class ConfigFileCreated(RuntimeError):
    pass


# ==========================================================
# 彩色时间输出
# ==========================================================

COLOR_RESET = "\033[0m"
COLOR_DIM = "\033[2m"
COLOR_RED = "\033[31m"
COLOR_GREEN = "\033[32m"
COLOR_YELLOW = "\033[33m"
COLOR_BLUE = "\033[34m"
COLOR_CYAN = "\033[36m"


def now_text() -> str:
    return time.strftime("%Y-%m-%d %H:%M:%S")


def _pick_color(text: str, file: Any) -> str:
    if file is sys.stderr:
        return COLOR_RED
    stripped = text.strip()
    if stripped.startswith("$ "):
        return COLOR_BLUE
    if "错误" in stripped or "失败" in stripped:
        return COLOR_RED
    if "警告" in stripped:
        return COLOR_YELLOW
    if "完成" in stripped or "通过" in stripped or "已创建" in stripped or "已更新" in stripped:
        return COLOR_GREEN
    return COLOR_CYAN


def print(
    *args: Any,
    sep: str = " ",
    end: str = "\n",
    file: Any = None,
    flush: bool = False,
    color: Optional[str] = None,
) -> None:
    target = file or sys.stdout
    message = sep.join(str(arg) for arg in args)

    if message == "":
        builtins.print("", end=end, file=target, flush=flush)
        return

    chosen_color = color or _pick_color(message, target)
    prefix = f"{COLOR_DIM}[{now_text()}]{COLOR_RESET} "
    lines = message.splitlines() or [""]
    for index, line in enumerate(lines):
        current_end = end if index == len(lines) - 1 else "\n"
        builtins.print(f"{prefix}{chosen_color}{line}{COLOR_RESET}", end=current_end, file=target, flush=flush)


@dataclass(frozen=True)
class DeployContext:
    root_domain: str
    email: str
    dns_provider: str
    public_ip: str
    frp_version: str
    bind_port: int
    dashboard_port: int
    status_port: int
    token: str
    dashboard_user: str
    dashboard_password: str
    suffix: str


# ==========================================================
# 通用工具函数
# ==========================================================

def eprint(*args: Any) -> None:
    print(*args, file=sys.stderr)


def prompt_input(message: str) -> str:
    print(message, end="", color=COLOR_YELLOW)
    return builtins.input()


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="FRPS + Nginx + Certbot 自动部署脚本")
    parser.add_argument(
        "-c",
        "--config",
        default=str(DEFAULT_CONFIG_FILE),
        help=f"JSON 配置文件路径，默认：{DEFAULT_CONFIG_FILE}",
    )
    parser.add_argument(
        "-r",
        "--run",
        action="store_true",
        help="生成相关文件后继续执行 DNS 检查、证书申请和容器启动；默认只生成文件",
    )
    return parser.parse_args()


def set_config_file(path_text: str) -> None:
    global CONFIG_FILE

    path = Path(path_text).expanduser()
    if not path.is_absolute():
        path = Path.cwd() / path
    CONFIG_FILE = path.resolve()


def clone_default_config() -> Dict[str, Any]:
    return json.loads(json.dumps(DEFAULT_CONFIG, ensure_ascii=False))


def write_default_config() -> None:
    data = clone_default_config()
    CONFIG_FILE.parent.mkdir(parents=True, exist_ok=True)
    CONFIG_FILE.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    try:
        CONFIG_FILE.chmod(0o600)
    except OSError:
        pass
    print(f"未找到配置文件，已生成默认配置：{CONFIG_FILE}")
    print("请按需填写 root_domain、cert_email、cf_api_token、vps_public_ip 和 services。")


def load_config_file() -> Dict[str, Any]:
    if not CONFIG_FILE.exists():
        write_default_config()
        raise ConfigFileCreated(str(CONFIG_FILE))

    try:
        raw = json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"配置文件不是合法 JSON：{CONFIG_FILE}，{exc}") from exc

    if not isinstance(raw, dict):
        raise ValueError(f"配置文件顶层必须是 JSON object：{CONFIG_FILE}")

    data = clone_default_config()
    data.update(raw)
    return data


def load_runtime_config() -> None:
    global CONFIG, SERVICES, ROOT_DOMAIN, CERT_EMAIL, CF_API_TOKEN, VPS_PUBLIC_IP, CF_ZONE_ID

    CONFIG = load_config_file()

    services = CONFIG.get("services", DEFAULT_SERVICES)
    if not isinstance(services, list):
        raise ValueError("配置项 services 必须是数组")

    SERVICES = services
    ROOT_DOMAIN = str(CONFIG.get("root_domain") or CONFIG.get("domain") or "").strip().lower()
    CERT_EMAIL = str(
        CONFIG.get("cert_email")
        or CONFIG.get("certificate_email")
        or CONFIG.get("email")
        or ""
    ).strip()
    CF_API_TOKEN = str(CONFIG.get("cf_api_token") or CONFIG.get("CF_API_TOKEN") or "").strip()
    CF_ZONE_ID = str(CONFIG.get("cf_zone_id") or CONFIG.get("CF_ZONE_ID") or "").strip()
    VPS_PUBLIC_IP = str(CONFIG.get("vps_public_ip") or CONFIG.get("VPS_PUBLIC_IP") or "").strip()


def run(cmd: List[str], check: bool = True, cwd: Path | None = None) -> subprocess.CompletedProcess[str]:
    print(f"\n$ {' '.join(cmd)}")
    return subprocess.run(cmd, check=check, cwd=str(cwd) if cwd else None, text=True)


def current_uid_gid() -> str:
    return f"{os.getuid()}:{os.getgid()}"


def capture(cmd: List[str], check: bool = True) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        cmd,
        check=check,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def command_exists(name: str) -> bool:
    return shutil.which(name) is not None


def toml_str(value: str) -> str:
    # TOML 字符串可直接复用 JSON 字符串转义
    return json.dumps(value, ensure_ascii=False)


def safe_alias(alias: str) -> str:
    # frp proxy name 和 nginx 文件名使用，保守一点
    allowed = string.ascii_letters + string.digits + "_-"
    return "".join(c if c in allowed else "_" for c in alias)


def random_letters(n: int = 16) -> str:
    return "".join(secrets.choice(string.ascii_letters) for _ in range(n))


def random_password(n: int = 16) -> str:
    chars = string.ascii_letters + string.digits + "!@#$%^&*()-_=+"
    while True:
        pwd = "".join(secrets.choice(chars) for _ in range(n))
        if (
            any(c.islower() for c in pwd)
            and any(c.isupper() for c in pwd)
            and any(c.isdigit() for c in pwd)
            and any(c in "!@#$%^&*()-_=+" for c in pwd)
        ):
            return pwd


def is_port_free(port: int, host: str = "127.0.0.1") -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.3)
        return s.connect_ex((host, port)) != 0


def random_free_port(start: int = 20000, end: int = 60000) -> int:
    for _ in range(200):
        port = random.randint(start, end)
        if is_port_free(port):
            return port
    raise RuntimeError("无法找到可用随机端口")


def random_free_port_excluding(used: set[int], start: int = 20000, end: int = 60000) -> int:
    for _ in range(200):
        port = random_free_port(start, end)
        if port not in used:
            used.add(port)
            return port
    raise RuntimeError("无法找到不冲突的可用随机端口")


def validate_ipv4(ip: str) -> str:
    try:
        parsed = ipaddress.ip_address(ip)
    except ValueError as exc:
        raise ValueError(f"不是合法 IP 地址：{ip}") from exc
    if parsed.version != 4:
        raise ValueError(f"当前脚本只自动创建 A 记录，请输入 IPv4 地址：{ip}")
    return str(parsed)


# ==========================================================
# 服务配置辅助函数
# ==========================================================

def mode_of(item: Dict[str, Any]) -> str:
    return str(item.get("mode", "http")).strip().lower()


def remote_port(item: Dict[str, Any]) -> int:
    return int(item["port"])


def local_port(item: Dict[str, Any]) -> int:
    return int(item.get("local_port", item["port"]))


def local_ip(item: Dict[str, Any]) -> str:
    return str(item.get("local_ip", "127.0.0.1"))


def http_services() -> List[Dict[str, Any]]:
    return [s for s in SERVICES if mode_of(s) == "http"]


def tcp_services() -> List[Dict[str, Any]]:
    return [s for s in SERVICES if mode_of(s) == "tcp"]


def all_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in SERVICES})


def http_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in http_services()})


def tcp_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in tcp_services()})


def validate_services() -> None:
    aliases = set()
    ports = set()

    for item in SERVICES:
        alias = str(item.get("alias", "")).strip()
        if not alias:
            raise ValueError("SERVICES 中存在空 alias")
        if alias in aliases:
            raise ValueError(f"SERVICES 中 alias 重复：{alias}")
        aliases.add(alias)

        mode = mode_of(item)
        if mode not in {"http", "tcp"}:
            raise ValueError(f"服务 {alias} 的 mode 非法：{mode}，只支持 http/tcp")

        rp = remote_port(item)
        lp = local_port(item)
        if not (1 <= rp <= 65535):
            raise ValueError(f"服务 {alias} 的 port 非法：{rp}")
        if not (1 <= lp <= 65535):
            raise ValueError(f"服务 {alias} 的 local_port 非法：{lp}")

        if rp in ports:
            raise ValueError(f"SERVICES 中 port 重复：{rp}")
        ports.add(rp)

        if mode == "http":
            # 子域名前缀尽量保持 DNS 兼容：字母、数字、短横线
            for ch in alias:
                if not (ch.isalnum() or ch == "-"):
                    raise ValueError(
                        f"HTTP 服务 alias 需要能作为子域名前缀，当前非法：{alias}。"
                        "建议使用 emby/test/panel 这类字母数字短横线。"
                    )


# ==========================================================
# 环境检查
# ==========================================================

def check_docker_compose() -> None:
    if not command_exists("docker"):
        eprint("错误：未找到 docker 命令，请先安装 Docker。")
        sys.exit(1)

    ret = subprocess.run(
        ["docker", "compose", "version"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
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
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
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


def check_required_ports() -> None:
    # 初始证书申请需要 80，正式访问需要 443。
    # 如果已有 nginx/apache/caddy 占用，脚本会失败。
    for p in (80, 443):
        if not is_port_free(p, host="127.0.0.1"):
            eprint(f"警告：本机端口 {p} 可能已被占用，后续 docker compose 可能启动失败。")
            eprint("如果你已经有宝塔/Nginx/Caddy 占用 80/443，需要先停掉，或改成接入现有反代。")


def get_latest_frp_version() -> str:
    url = "https://api.github.com/repos/fatedier/frp/releases/latest"
    try:
        req = urllib.request.Request(
            url,
            headers={"User-Agent": "frp-stack-deploy-script"},
        )
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
    configured_ip = VPS_PUBLIC_IP.strip() or os.getenv("VPS_PUBLIC_IP", "").strip()
    if configured_ip:
        return validate_ipv4(configured_ip)

    urls = [
        "https://api.ipify.org?format=json",
        "https://ifconfig.co/json",
    ]

    for url in urls:
        try:
            req = urllib.request.Request(url, headers={"User-Agent": "frp-stack-deploy-script"})
            with urllib.request.urlopen(req, timeout=8) as resp:
                data = json.loads(resp.read().decode("utf-8"))
                ip = str(data.get("ip") or data.get("ip_addr") or "").strip()
                if ip:
                    ip = validate_ipv4(ip)
                    print(f"检测到当前 VPS 公网 IP：{ip}")
                    answer = prompt_input("VPS_PUBLIC_IP 为空，是否使用该公网 IP 继续？[Y/n]：").strip().lower()
                    if answer and answer not in {"y", "yes"}:
                        print("用户取消部署。")
                        sys.exit(0)
                    return ip
        except Exception:
            pass

    ip = prompt_input("自动获取公网 IP 失败，请手动输入 VPS 公网 IPv4：").strip()
    return validate_ipv4(ip)


# ==========================================================
# DNS 自动解析：Cloudflare
# ==========================================================

def cf_request(
    method: str,
    path: str,
    token: str,
    payload: Optional[Dict[str, Any]] = None,
    query: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    base_url = "https://api.cloudflare.com/client/v4"
    url = base_url + path
    if query:
        url += "?" + urllib.parse.urlencode(query)

    data_bytes = None
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "User-Agent": "frp-stack-deploy-script",
    }
    if payload is not None:
        data_bytes = json.dumps(payload).encode("utf-8")

    req = urllib.request.Request(url, data=data_bytes, headers=headers, method=method.upper())

    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            raw = resp.read().decode("utf-8")
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"Cloudflare API HTTP {exc.code}: {raw}") from exc

    try:
        body = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise RuntimeError(f"Cloudflare API 返回非 JSON：{raw[:300]}") from exc

    if not body.get("success"):
        raise RuntimeError(f"Cloudflare API 调用失败：{json.dumps(body, ensure_ascii=False)}")

    return body


def cloudflare_get_zone_id(root_domain: str, token: str) -> str:
    configured_zone_id = CF_ZONE_ID.strip() or os.getenv("CF_ZONE_ID", "").strip()
    if configured_zone_id:
        print(f"使用已配置 CF_ZONE_ID：{configured_zone_id}")
        return configured_zone_id

    body = cf_request(
        "GET",
        "/zones",
        token,
        query={"name": root_domain, "status": "active", "per_page": 50},
    )
    result = body.get("result") or []
    if not result:
        raise RuntimeError(f"Cloudflare 未找到 Zone：{root_domain}。请确认域名 DNS 已托管到 Cloudflare。")

    zone_id = str(result[0]["id"])
    print(f"Cloudflare Zone ID：{zone_id}")
    return zone_id


def cloudflare_find_a_record(zone_id: str, token: str, name: str) -> Optional[Dict[str, Any]]:
    body = cf_request(
        "GET",
        f"/zones/{zone_id}/dns_records",
        token,
        query={"type": "A", "name": name, "per_page": 100},
    )
    records = body.get("result") or []
    if records:
        return records[0]
    return None


def cloudflare_upsert_a_record(zone_id: str, token: str, name: str, ip: str) -> None:
    payload = {
        "type": "A",
        "name": name,
        "content": ip,
        "ttl": 60,
        "proxied": False,
    }

    record = cloudflare_find_a_record(zone_id, token, name)
    if record:
        record_id = record["id"]
        old_ip = record.get("content")
        if old_ip == ip and record.get("proxied") is False:
            print(f"DNS 已存在且正确：{name} -> {ip}")
            return
        cf_request("PATCH", f"/zones/{zone_id}/dns_records/{record_id}", token, payload=payload)
        print(f"DNS 已更新：{name} {old_ip} -> {ip}")
    else:
        cf_request("POST", f"/zones/{zone_id}/dns_records", token, payload=payload)
        print(f"DNS 已创建：{name} -> {ip}")


def setup_dns_cloudflare(ctx: DeployContext) -> None:
    if not http_services():
        print("没有 http 模式服务，跳过 DNS 自动解析。")
        return

    token = CF_API_TOKEN.strip() or os.getenv("CF_API_TOKEN", "").strip()
    if not token:
        raise RuntimeError(f"未配置 cf_api_token。请先将 Cloudflare API Token 填入配置文件：{CONFIG_FILE}")

    zone_id = cloudflare_get_zone_id(ctx.root_domain, token)
    for domain in http_domains(ctx.root_domain):
        cloudflare_upsert_a_record(zone_id, token, domain, ctx.public_ip)


def setup_dns(ctx: DeployContext) -> None:
    if ctx.dns_provider == DNS_PROVIDER_MANUAL:
        print_dns_notice(ctx)
        if http_services():
            prompt_input("\n确认 DNS 已配置并生效后，按回车继续；否则 Ctrl+C 退出...")
        return

    if ctx.dns_provider == DNS_PROVIDER_CLOUDFLARE:
        setup_dns_cloudflare(ctx)
        wait_dns_records(ctx)
        return

    raise ValueError(f"不支持的 DNS provider：{ctx.dns_provider}")


def resolve_a_records(domain: str) -> List[str]:
    try:
        infos = socket.getaddrinfo(domain, None, family=socket.AF_INET, type=socket.SOCK_STREAM)
        return sorted({info[4][0] for info in infos})
    except socket.gaierror:
        return []


def wait_dns_records(ctx: DeployContext, timeout_seconds: int = 240, interval_seconds: int = 10) -> None:
    domains = http_domains(ctx.root_domain)
    if not domains:
        return

    print("\n等待 DNS 生效...")
    deadline = time.time() + timeout_seconds
    pending = set(domains)

    while pending and time.time() < deadline:
        for domain in list(pending):
            ips = resolve_a_records(domain)
            if ctx.public_ip in ips:
                print(f"DNS 已生效：{domain} -> {ctx.public_ip}")
                pending.remove(domain)
            else:
                print(f"DNS 未生效：{domain} 当前解析 {ips or '无结果'}，期望 {ctx.public_ip}")
        if pending:
            time.sleep(interval_seconds)

    if pending:
        print("\n警告：以下域名本地解析仍未确认生效，证书申请可能失败：")
        for domain in sorted(pending):
            print(f"  {domain}")
        prompt_input("可以继续尝试申请证书，按回车继续；否则 Ctrl+C 退出...")


# ==========================================================
# 文件生成
# ==========================================================

def ensure_dirs() -> None:
    NGINX_CONF_DIR.mkdir(parents=True, exist_ok=True)
    CERTBOT_CONF_DIR.mkdir(parents=True, exist_ok=True)
    CERTBOT_WWW_DIR.mkdir(parents=True, exist_ok=True)
    STATUS_APP_DATA_DIR.mkdir(parents=True, exist_ok=True)
    FRPC_BASE_DIR.mkdir(parents=True, exist_ok=True)


def generate_frps_toml(ctx: DeployContext) -> None:
    lines: List[str] = []
    lines.append('bindAddr = "0.0.0.0"')
    lines.append(f"bindPort = {ctx.bind_port}")
    lines.append("")
    lines.append("[auth]")
    lines.append('method = "token"')
    lines.append(f"token = {toml_str(ctx.token)}")
    lines.append("")
    lines.append("[transport.tls]")
    lines.append("force = true")
    lines.append("")
    lines.append("[webServer]")
    lines.append('addr = "0.0.0.0"')
    lines.append(f"port = {ctx.dashboard_port}")
    lines.append(f"user = {toml_str(ctx.dashboard_user)}")
    lines.append(f"password = {toml_str(ctx.dashboard_password)}")
    lines.append("")

    for port in all_remote_ports():
        lines.append("[[allowPorts]]")
        lines.append(f"single = {port}")
        lines.append("")

    FRPS_TOML_FILE.write_text("\n".join(lines), encoding="utf-8")


def generate_frpc_toml(ctx: DeployContext) -> None:
    lines: List[str] = []
    lines.append(f'serverAddr = "{ctx.public_ip}"')
    lines.append(f"serverPort = {ctx.bind_port}")
    lines.append("")
    lines.append("[auth]")
    lines.append('method = "token"')
    lines.append(f"token = {toml_str(ctx.token)}")
    lines.append("")
    lines.append("[transport.tls]")
    lines.append("enable = true")

    for item in SERVICES:
        lines.append("")
        lines.append(f"# {item.get('comment', item['alias'])}")
        lines.append("[[proxies]]")
        lines.append(f"name = {toml_str(str(item['alias']))}")
        lines.append('type = "tcp"')
        lines.append(f"localIP = {toml_str(local_ip(item))}")
        lines.append(f"localPort = {local_port(item)}")
        lines.append(f"remotePort = {remote_port(item)}")

    FRPC_TOML_FILE.write_text("\n".join(lines) + "\n", encoding="utf-8")


def generate_frps_compose(ctx: DeployContext) -> None:
    uid_gid = current_uid_gid()
    port_lines: List[str] = [
        f'      - "{ctx.bind_port}:{ctx.bind_port}/tcp"',
        f'      - "127.0.0.1:{ctx.dashboard_port}:{ctx.dashboard_port}/tcp"',
    ]

    # tcp 模式服务直接暴露到公网
    for p in tcp_remote_ports():
        port_lines.append(f'      - "{p}:{p}/tcp"')

    ports_block = "\n".join(port_lines)

    expose_section = ""
    if http_remote_ports():
        expose_lines = "\n".join(f'      - "{p}"' for p in http_remote_ports())
        expose_section = f"""
    expose:
{expose_lines}
""".rstrip()

    # 如果没有 http 服务，nginx/certbot 其实可以不启；但保留模板会占用 80/443。
    # 这里按你的需求默认支持反代，所以保留 nginx/certbot。
    compose = f"""
services:
  frps:
    image: fatedier/frps:{ctx.frp_version}
    container_name: frps_{ctx.suffix}
    user: "{uid_gid}"
    restart: unless-stopped
    volumes:
      - ./frps.toml:/etc/frp/frps.toml:ro
    command: ["-c", "/etc/frp/frps.toml"]
    ports:
{ports_block}
{expose_section if expose_section else ""}

  nginx:
    image: nginx:1.27-alpine
    container_name: nginx_{ctx.suffix}
    restart: unless-stopped
    depends_on:
      - frps
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "80:80/tcp"
      - "443:443/tcp"
    volumes:
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./certbot/www:/var/www/certbot:ro
      - ./certbot/conf:/etc/letsencrypt:ro
    command: >
      /bin/sh -c "while :; do sleep 6h; nginx -s reload || true; done &
      nginx -g 'daemon off;'"

  certbot:
    image: certbot/certbot:latest
    container_name: certbot_{ctx.suffix}
    user: "{uid_gid}"
    restart: unless-stopped
    volumes:
      - ./certbot/www:/var/www/certbot
      - ./certbot/conf:/etc/letsencrypt
    entrypoint: >
      /bin/sh -c "trap exit TERM;
      while :; do
        certbot renew --webroot -w /var/www/certbot --quiet;
        sleep 12h & wait $${{!}};
      done"

""".strip()

    COMPOSE_FILE.write_text(compose + "\n", encoding="utf-8")


def generate_frpc_compose(ctx: DeployContext) -> None:
    uid_gid = current_uid_gid()
    compose = f"""
services:
  frpc:
    image: fatedier/frpc:{ctx.frp_version}
    container_name: frpc_{ctx.suffix}
    user: "{uid_gid}"
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./frpc.toml:/etc/frp/frpc.toml:ro
    command: ["-c", "/etc/frp/frpc.toml"]
""".strip()

    FRPC_COMPOSE_FILE.write_text(compose + "\n", encoding="utf-8")


def http_domains(root_domain: str) -> List[str]:
    return [f"{item['alias']}.{root_domain}" for item in http_services()]


def generate_http_challenge_conf(ctx: DeployContext) -> None:
    domains = http_domains(ctx.root_domain)
    if not domains:
        return

    server_names = " ".join(domains)
    conf = f"""
server {{
    listen 80;
    server_name {server_names};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

    location = /_frps-status {{
        return 301 /_frps-status/;
    }}

    location /_frps-status/ {{
        proxy_pass http://host.docker.internal:{ctx.status_port}/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}

    location / {{
        return 200 "certbot challenge server is running\\n";
        add_header Content-Type text/plain;
    }}
}}
""".strip()

    (NGINX_CONF_DIR / "00-http-challenge.conf").write_text(conf + "\n", encoding="utf-8")


def generate_https_nginx_confs(ctx: DeployContext) -> None:
    for item in http_services():
        alias = str(item["alias"])
        domain = f"{alias}.{ctx.root_domain}"
        port = remote_port(item)
        comment = str(item.get("comment", alias))
        filename = f"{safe_alias(alias)}.conf"

        conf = f"""
# {comment}
server {{
    listen 80;
    server_name {domain};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

    location = /_frps-status {{
        return 301 /_frps-status/;
    }}

    location /_frps-status/ {{
        proxy_pass http://host.docker.internal:{ctx.status_port}/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }}

    location / {{
        return 301 https://$host$request_uri;
    }}
}}

server {{
    listen 443 ssl http2;
    server_name {domain};

    ssl_certificate /etc/letsencrypt/live/{domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{domain}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    client_max_body_size 0;

    location = /_frps-status {{
        return 301 /_frps-status/;
    }}

    location /_frps-status/ {{
        proxy_pass http://host.docker.internal:{ctx.status_port}/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }}

    location / {{
        proxy_pass http://frps:{port};

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }}
}}
""".strip()

        (NGINX_CONF_DIR / filename).write_text(conf + "\n", encoding="utf-8")


def remove_https_confs() -> None:
    for item in http_services():
        p = NGINX_CONF_DIR / f"{safe_alias(str(item['alias']))}.conf"
        if p.exists():
            p.unlink()


def write_generated_info(ctx: DeployContext) -> None:
    lines = [
        f"ROOT_DOMAIN={ctx.root_domain}",
        f"EMAIL={ctx.email}",
        f"DNS_PROVIDER={ctx.dns_provider}",
        f"VPS_PUBLIC_IP={ctx.public_ip}",
        f"FRP_VERSION={ctx.frp_version}",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"STATUS_PORT={ctx.status_port}",
        f"STATUS_LOCAL_URL=http://127.0.0.1:{ctx.status_port}",
        f"FRPS_TOKEN={ctx.token}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"CONTAINER_SUFFIX={ctx.suffix}",
        f"DOCKER_USER={current_uid_gid()}",
        f"FRPS_DIR={BASE_DIR}",
        f"FRPC_DIR={FRPC_BASE_DIR}",
        "",
        "# HTTP services",
    ]

    for item in http_services():
        lines.append(
            f"HTTP_{item['alias'].upper()}=https://{item['alias']}.{ctx.root_domain} "
            f"remotePort={remote_port(item)} local={local_ip(item)}:{local_port(item)}"
        )

    lines.append("")
    lines.append("# TCP services")
    for item in tcp_services():
        lines.append(
            f"TCP_{str(item['alias']).upper()}={ctx.public_ip}:{remote_port(item)} "
            f"local={local_ip(item)}:{local_port(item)}"
        )

    GENERATED_INFO_FILE.write_text("\n".join(lines) + "\n", encoding="utf-8")


# ==========================================================
# 证书申请与部署流程
# ==========================================================

def issue_certs(ctx: DeployContext) -> None:
    if not http_services():
        print("没有 http 模式服务，跳过证书申请。")
        return

    for item in http_services():
        domain = f"{item['alias']}.{ctx.root_domain}"
        print(f"\n开始申请证书：{domain}")
        verify_http_challenge(domain)
        certbot_cmd = [
            "docker", "compose", "run", "--rm",
            "--entrypoint", "certbot",
            "certbot",
            "certonly",
            "--webroot",
            "-w", "/var/www/certbot",
            "-d", domain,
            "--email", ctx.email,
            "--agree-tos",
            "--no-eff-email",
            "--non-interactive",
        ]

        ret = run(certbot_cmd, cwd=BASE_DIR, check=False)
        if ret.returncode == 0:
            continue

        print("certbot 首次申请失败，等待 15 秒后重试一次...")
        time.sleep(15)
        run(certbot_cmd, cwd=BASE_DIR)


def docker_compose_up_initial() -> None:
    # 初次启动 nginx 只使用 challenge 配置，避免证书文件不存在导致 nginx 启动失败。
    run(["docker", "compose", "up", "-d", "frps", "nginx"], cwd=BASE_DIR)


def docker_compose_restart_nginx() -> None:
    run(["docker", "compose", "restart", "nginx"], cwd=BASE_DIR)


def docker_compose_up_all() -> None:
    run(["docker", "compose", "up", "-d"], cwd=BASE_DIR)


def verify_http_challenge(domain: str) -> None:
    token = f"frps-self-check-{secrets.token_hex(8)}"
    token_dir = CERTBOT_WWW_DIR / ".well-known" / "acme-challenge"
    token_dir.mkdir(parents=True, exist_ok=True)
    token_file = token_dir / token
    token_file.write_text(token, encoding="utf-8")

    url = f"http://{domain}/.well-known/acme-challenge/{token}"
    try:
        with urllib.request.urlopen(url, timeout=15) as resp:
            body = resp.read().decode("utf-8", errors="replace").strip()
    except Exception as exc:
        raise RuntimeError(f"HTTP-01 自检失败：无法访问 {url}，原因：{exc}") from exc
    finally:
        try:
            token_file.unlink()
        except FileNotFoundError:
            pass

    if body != token:
        raise RuntimeError(f"HTTP-01 自检失败：{url} 返回内容不匹配，实际返回：{body[:120]!r}")


# ==========================================================
# 输出结果
# ==========================================================

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

    for item in SERVICES:
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
            print(
                f"  {item.get('comment', item['alias'])}: "
                f"{ctx.public_ip}:{remote_port(item)} -> {local_ip(item)}:{local_port(item)}"
            )

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
    print("需要继续执行部署启动时，请重新运行：")
    print(f"  {Path(sys.argv[0]).name} -c {CONFIG_FILE} -r")

    print_dns_notice(ctx)
    print_frpc_config(ctx)


# ==========================================================
# 主流程
# ==========================================================

def prompt_user() -> Tuple[str, str, str]:
    root_domain = ROOT_DOMAIN
    if root_domain:
        print(f"使用配置文件中的主域名：{root_domain}")
    else:
        root_domain = prompt_input("请输入主域名，例如 hello.com：").strip().lower()
    if not root_domain or "." not in root_domain:
        eprint("域名格式不正确。")
        sys.exit(1)

    email = CERT_EMAIL
    if email:
        print(f"使用配置文件中的证书邮箱：{email}")
    else:
        email = prompt_input("请输入证书邮箱：").strip()
    if "@" not in email:
        eprint("邮箱格式不正确。")
        sys.exit(1)

    configured_default_provider = DNS_PROVIDER_CLOUDFLARE if CF_API_TOKEN.strip() else DNS_PROVIDER_MANUAL
    configured_provider = str(CONFIG.get("dns_provider") or "").strip().lower()
    default_provider = (
        configured_provider
        or os.getenv("DNS_PROVIDER", configured_default_provider).strip().lower()
        or configured_default_provider
    )
    if default_provider not in SUPPORTED_DNS_PROVIDERS:
        default_provider = DNS_PROVIDER_MANUAL

    if configured_provider:
        provider = default_provider
        print(f"使用配置文件中的 DNS 解析方式：{provider}")
    else:
        provider = prompt_input(
            f"请选择 DNS 解析方式 [{DNS_PROVIDER_MANUAL}/{DNS_PROVIDER_CLOUDFLARE}]，默认 {default_provider}："
        ).strip().lower() or default_provider

    if provider not in SUPPORTED_DNS_PROVIDERS:
        eprint(f"不支持的 DNS 解析方式：{provider}")
        sys.exit(1)

    return root_domain, email, provider


def build_context(root_domain: str, email: str, dns_provider: str) -> DeployContext:
    public_ip = get_public_ip() if http_services() else (VPS_PUBLIC_IP.strip() or os.getenv("VPS_PUBLIC_IP", "你的VPS公网IP"))
    generated_ports: set[int] = set(all_remote_ports())
    bind_port = random_free_port_excluding(generated_ports)
    dashboard_port = random_free_port_excluding(generated_ports)
    status_port = random_free_port_excluding(generated_ports)
    return DeployContext(
        root_domain=root_domain,
        email=email,
        dns_provider=dns_provider,
        public_ip=public_ip,
        frp_version=get_latest_frp_version(),
        bind_port=bind_port,
        dashboard_port=dashboard_port,
        status_port=status_port,
        token=random_password(32),
        dashboard_user="admin",
        dashboard_password=random_password(16),
        suffix=random_letters(16),
    )


def write_status_app_env(ctx: DeployContext) -> None:
    STATUS_APP_DIR.mkdir(parents=True, exist_ok=True)
    STATUS_APP_DATA_DIR.mkdir(parents=True, exist_ok=True)
    lines = [
        f"DOCKER_USER={current_uid_gid()}",
        f"LISTEN=127.0.0.1:{ctx.status_port}",
        "DB_PATH=/data/frps-status.sqlite",
        "FRPS_HOST=127.0.0.1",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"FRPS_DASHBOARD_USER={ctx.dashboard_user}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"STATUS_DOMAINS={','.join(http_domains(ctx.root_domain))}",
        f"STATUS_USER={ctx.dashboard_user}",
        f"STATUS_PASSWORD={ctx.dashboard_password}",
        "CERT_DIR=/etc/letsencrypt/live",
        "POLL_SECONDS=60",
        f"FRPS_CERTBOT_CONF_DIR={CERTBOT_CONF_DIR}",
        "",
    ]
    STATUS_APP_ENV_FILE.write_text("\n".join(lines), encoding="utf-8")


def docker_compose_up_status_app() -> None:
    run(["docker", "compose", "build"], cwd=STATUS_APP_DIR)
    run(["docker", "compose", "up", "-d"], cwd=STATUS_APP_DIR)


def generate_files(ctx: DeployContext) -> None:
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

    print("写入 frps-status-app/.env...")
    write_status_app_env(ctx)

    print("\n生成临时 HTTP challenge 配置...")
    remove_https_confs()
    generate_http_challenge_conf(ctx)


def main() -> None:
    args = parse_args()
    print("=== FRPS + Nginx + Certbot 自动部署脚本 ===")
    set_config_file(args.config)
    print(f"使用配置文件：{CONFIG_FILE}")

    try:
        load_runtime_config()
        validate_services()
    except ConfigFileCreated:
        print("已只生成默认配置文件，本次不执行部署。")
        sys.exit(0)
    except Exception as exc:
        eprint(f"配置错误：{exc}")
        sys.exit(1)

    root_domain, email, dns_provider = prompt_user()
    ctx = build_context(root_domain, email, dns_provider)

    generate_files(ctx)

    if not args.run:
        print_generate_only_result(ctx)
        return

    check_docker_compose()
    check_docker_permission()
    check_required_ports()

    setup_dns(ctx)

    print("\n启动 frps + nginx，用于证书 HTTP 验证...")
    docker_compose_up_initial()

    print("\n申请 HTTPS 证书...")
    issue_certs(ctx)

    print("\n生成正式 HTTPS Nginx 反代配置...")
    generate_https_nginx_confs(ctx)

    print("\n重启 Nginx 应用 HTTPS 配置...")
    docker_compose_restart_nginx()

    print("\n启动 certbot 自动续期容器...")
    docker_compose_up_all()

    print("\n编译并启动状态服务工程...")
    docker_compose_up_status_app()

    print_result(ctx)


if __name__ == "__main__":
    main()
