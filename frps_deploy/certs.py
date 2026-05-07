"""证书申请与 Docker Compose 容器操作。"""
from __future__ import annotations

import secrets
import time
import urllib.request
from typing import Optional

from frps_deploy.console import print
from frps_deploy.constants import BASE_DIR, CERTBOT_CONF_DIR, CERTBOT_WWW_DIR
from frps_deploy.models import DeployContext
from frps_deploy.services import managed_domains
from frps_deploy.utils import capture, run


def _read_url(url: str, host_header: Optional[str] = None, timeout: int = 15) -> str:
    req = urllib.request.Request(url)
    if host_header:
        req.add_header("Host", host_header)
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return resp.read().decode("utf-8", errors="replace").strip()


def verify_http_challenge(domain: str) -> None:
    token = f"frps-self-check-{secrets.token_hex(8)}"
    token_dir = CERTBOT_WWW_DIR / ".well-known" / "acme-challenge"
    token_dir.mkdir(parents=True, exist_ok=True)
    token_file = token_dir / token
    token_file.write_text(token, encoding="utf-8")

    path = f"/.well-known/acme-challenge/{token}"
    local_url = f"http://127.0.0.1{path}"
    public_url = f"http://{domain}{path}"
    try:
        try:
            local_body = _read_url(local_url, host_header=domain, timeout=8)
        except Exception as exc:
            ps = capture(["docker", "compose", "ps"], cwd=BASE_DIR, check=False)
            logs = capture(["docker", "compose", "logs", "--tail", "80", "nginx"], cwd=BASE_DIR, check=False)
            detail = [
                f"本机 Nginx HTTP-01 自检失败：无法访问 {local_url}，请求 Host 头为 {domain}，原因：{exc}",
                "这通常表示 nginx 容器未启动、80 端口未映射成功，或 nginx 配置加载失败。",
            ]
            if ps.stdout.strip():
                detail.append("\n【docker compose ps 标准输出】\n" + ps.stdout.strip())
            if ps.stderr.strip():
                detail.append("\n【docker compose ps 标准错误】\n" + ps.stderr.strip())
            if logs.stdout.strip():
                detail.append("\n【nginx 容器日志 标准输出】\n" + logs.stdout.strip())
            if logs.stderr.strip():
                detail.append("\n【nginx 容器日志 标准错误】\n" + logs.stderr.strip())
            raise RuntimeError("\n".join(detail)) from exc

        if local_body != token:
            raise RuntimeError(
                f"本机 Nginx HTTP-01 自检失败：{local_url} 返回内容不匹配，"
                f"请求 Host 头为 {domain}，实际返回：{local_body[:120]!r}"
            )

        body = _read_url(public_url, timeout=15)
    except Exception as exc:
        if isinstance(exc, RuntimeError):
            raise
        raise RuntimeError(
            f"HTTP-01 公网自检失败：无法访问 {public_url}，原因：{exc}\n"
            "请检查：域名 A 记录是否指向当前 VPS、公网 80 端口是否放行、云厂商安全组/防火墙是否允许 TCP 80。"
        ) from exc
    finally:
        try:
            token_file.unlink()
        except FileNotFoundError:
            pass

    if body != token:
        raise RuntimeError(f"HTTP-01 公网自检失败：{public_url} 返回内容不匹配，实际返回：{body[:120]!r}")


def _cert_file_exists(path) -> bool:
    try:
        return path.exists()
    except PermissionError:
        return True


def fix_certbot_conf_permissions() -> None:
    try:
        exists = CERTBOT_CONF_DIR.exists()
    except PermissionError:
        exists = True
    if not exists:
        return

    print(f"\n修正 certbot 证书目录权限：{CERTBOT_CONF_DIR}")
    ret = run(
        [
            "docker", "run", "--rm",
            "-v", f"{CERTBOT_CONF_DIR}:/target",
            "alpine",
            "chmod", "-R", "777", "/target",
        ],
        check=False,
    )
    if ret.returncode != 0:
        print("警告：证书目录权限修正失败，后续证书检查可能仍会遇到权限问题。")


def issue_certs(ctx: DeployContext) -> None:
    domains = managed_domains(ctx.root_domain)
    if not domains:
        print("没有需要申请证书的域名，跳过证书申请。")
        return

    fix_certbot_conf_permissions()

    for domain in domains:
        cert_file = CERTBOT_CONF_DIR / "live" / domain / "fullchain.pem"
        key_file = CERTBOT_CONF_DIR / "live" / domain / "privkey.pem"
        if _cert_file_exists(cert_file) and _cert_file_exists(key_file):
            print(f"\n证书已存在，跳过申请：{domain}")
            continue
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
    run(["docker", "compose", "up", "-d", "--remove-orphans", "frps", "nginx"], cwd=BASE_DIR)
    time.sleep(2)
    ret = capture(["docker", "compose", "ps", "nginx"], cwd=BASE_DIR, check=False)
    if ret.returncode != 0:
        logs = capture(["docker", "compose", "logs", "--tail", "80", "nginx"], cwd=BASE_DIR, check=False)
        raise RuntimeError(
            "nginx 容器启动后状态检查失败。\n"
            f"【docker compose ps nginx】\n{ret.stdout}{ret.stderr}\n"
            f"【nginx 容器日志】\n{logs.stdout}{logs.stderr}"
        )


def docker_compose_restart_nginx() -> None:
    run(["docker", "compose", "restart", "nginx"], cwd=BASE_DIR)


def docker_compose_up_all() -> None:
    run(["docker", "compose", "up", "-d", "--remove-orphans"], cwd=BASE_DIR)
