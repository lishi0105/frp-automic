"""证书申请与 Docker Compose 容器操作。"""
from __future__ import annotations

import secrets
import time
import urllib.request

from frps_deploy.console import print
from frps_deploy.constants import BASE_DIR, CERTBOT_WWW_DIR, STATUS_APP_DIR
from frps_deploy.models import DeployContext
from frps_deploy.services import http_services
from frps_deploy.utils import run


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
            "--entrypoint", "certbot", "certbot",
            "certonly", "--webroot", "-w", "/var/www/certbot",
            "-d", domain,
            "--email", ctx.email,
            "--agree-tos", "--no-eff-email", "--non-interactive",
        ]
        ret = run(certbot_cmd, cwd=BASE_DIR, check=False)
        if ret.returncode == 0:
            continue
        print("certbot 首次申请失败，等待 15 秒后重试一次...")
        time.sleep(15)
        run(certbot_cmd, cwd=BASE_DIR)


def docker_compose_up_initial() -> None:
    run(["docker", "compose", "up", "-d", "frps", "nginx"], cwd=BASE_DIR)


def docker_compose_restart_nginx() -> None:
    run(["docker", "compose", "restart", "nginx"], cwd=BASE_DIR)


def docker_compose_up_all() -> None:
    run(["docker", "compose", "up", "-d"], cwd=BASE_DIR)


def docker_compose_up_status_app() -> None:
    run(["docker", "compose", "build"], cwd=STATUS_APP_DIR)
    run(["docker", "compose", "up", "-d"], cwd=STATUS_APP_DIR)
