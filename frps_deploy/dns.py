"""DNS 自动解析：Cloudflare API、DNS 生效等待。"""
from __future__ import annotations

import json
import socket
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional

from frps_deploy import config
from frps_deploy.console import print, prompt_input
from frps_deploy.constants import DNS_PROVIDER_CLOUDFLARE, DNS_PROVIDER_MANUAL
from frps_deploy.models import DeployContext
from frps_deploy.services import http_domains, http_services


def cf_request(
    method: str,
    path: str,
    token: str,
    payload: Optional[Dict[str, Any]] = None,
    query: Optional[Dict[str, Any]] = None,
) -> Dict[str, Any]:
    url = "https://api.cloudflare.com/client/v4" + path
    if query:
        url += "?" + urllib.parse.urlencode(query)

    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json",
        "User-Agent": "frp-stack-deploy-script",
    }
    data_bytes = json.dumps(payload).encode("utf-8") if payload is not None else None
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
    if config.CF_ZONE_ID.strip():
        print(f"使用已配置 CF_ZONE_ID：{config.CF_ZONE_ID}")
        return config.CF_ZONE_ID.strip()

    body = cf_request("GET", "/zones", token, query={"name": root_domain, "status": "active", "per_page": 50})
    result = body.get("result") or []
    if not result:
        raise RuntimeError(f"Cloudflare 未找到 Zone：{root_domain}。请确认域名 DNS 已托管到 Cloudflare。")
    zone_id = str(result[0]["id"])
    print(f"Cloudflare Zone ID：{zone_id}")
    return zone_id


def cloudflare_find_a_record(zone_id: str, token: str, name: str) -> Optional[Dict[str, Any]]:
    body = cf_request("GET", f"/zones/{zone_id}/dns_records", token, query={"type": "A", "name": name, "per_page": 100})
    records = body.get("result") or []
    return records[0] if records else None


def cloudflare_upsert_a_record(zone_id: str, token: str, name: str, ip: str) -> None:
    payload = {"type": "A", "name": name, "content": ip, "ttl": 60, "proxied": False}
    record = cloudflare_find_a_record(zone_id, token, name)
    if record:
        old_ip = record.get("content")
        if old_ip == ip and record.get("proxied") is False:
            print(f"DNS 已存在且正确：{name} -> {ip}")
            return
        cf_request("PATCH", f"/zones/{zone_id}/dns_records/{record['id']}", token, payload=payload)
        print(f"DNS 已更新：{name} {old_ip} -> {ip}")
    else:
        cf_request("POST", f"/zones/{zone_id}/dns_records", token, payload=payload)
        print(f"DNS 已创建：{name} -> {ip}")


def setup_dns_cloudflare(ctx: DeployContext) -> None:
    if not http_services():
        print("没有 http 模式服务，跳过 DNS 自动解析。")
        return
    token = config.CF_API_TOKEN.strip()
    if not token:
        raise RuntimeError(f"未配置 cf_api_token。请先将 Cloudflare API Token 填入配置文件：{config.CONFIG_FILE}")
    zone_id = cloudflare_get_zone_id(ctx.root_domain, token)
    for domain in http_domains(ctx.root_domain):
        cloudflare_upsert_a_record(zone_id, token, domain, ctx.public_ip)


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


def setup_dns(ctx: DeployContext) -> None:
    if ctx.dns_provider == DNS_PROVIDER_MANUAL:
        from frps_deploy.output import print_dns_notice
        print_dns_notice(ctx)
        if http_services():
            prompt_input("\n确认 DNS 已配置并生效后，按回车继续；否则 Ctrl+C 退出...")
        return
    if ctx.dns_provider == DNS_PROVIDER_CLOUDFLARE:
        setup_dns_cloudflare(ctx)
        wait_dns_records(ctx)
        return
    raise ValueError(f"不支持的 DNS provider：{ctx.dns_provider}")
