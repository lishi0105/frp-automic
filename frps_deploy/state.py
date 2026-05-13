"""已部署状态记录与配置差异计算。"""
from __future__ import annotations

import json
from typing import Any, Dict, List

from frps_deploy import config
from frps_deploy.constants import DEPLOY_STATE_FILE, GENERATED_INFO_FILE
from frps_deploy.models import DeployContext
from frps_deploy.services import (
    expose_http_port, local_ip, local_port, mode_of, needs_tunnel, remote_port,
)
from frps_deploy.utils import safe_alias


def _service_state(item: Dict[str, Any]) -> Dict[str, Any]:
    alias = str(item["alias"])
    state = {
        "alias": alias,
        "safe_alias": safe_alias(alias),
        "comment": str(item.get("comment", alias)),
        "mode": mode_of(item),
        "tunnel": needs_tunnel(item),
        "port": remote_port(item),
        "local_port": local_port(item),
        "local_ip": local_ip(item),
        "expose_http_port": expose_http_port(item),
    }
    return state


def _read_generated_info_state() -> Dict[str, Any]:
    if not GENERATED_INFO_FILE.exists():
        return {}
    values: Dict[str, str] = {}
    for line in GENERATED_INFO_FILE.read_text(encoding="utf-8").splitlines():
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    if not values:
        return {}

    def as_int(name: str, default: int = 0) -> int:
        try:
            return int(values.get(name, default) or default)
        except ValueError:
            return default

    return {
        "version": 1,
        "root_domain": values.get("ROOT_DOMAIN", ""),
        "email": values.get("EMAIL", ""),
        "frp_version": values.get("FRP_VERSION", ""),
        "status_app_version": values.get("STATUS_APP_VERSION", ""),
        "vps_public_ip": values.get("VPS_PUBLIC_IP", ""),
        "bind_port": as_int("FRPS_BIND_PORT"),
        "dashboard_port": as_int("FRPS_DASHBOARD_PORT"),
        "status_port": as_int("STATUS_PORT"),
        "token": values.get("FRPS_TOKEN", ""),
        "dashboard_user": "admin",
        "dashboard_password": values.get("FRPS_DASHBOARD_PASSWORD", ""),
        "frpc_use_encryption": values.get("FRPC_USE_ENCRYPTION", "").lower() in {"1", "true", "yes", "y", "on"},
        "frpc_use_compression": values.get("FRPC_USE_COMPRESSION", "").lower() in {"1", "true", "yes", "y", "on"},
        "frpc_tcp_mux": values.get("FRPC_TCP_MUX", "").lower() in {"1", "true", "yes", "y", "on"},
        "frpc_protocol": values.get("FRPC_PROTOCOL", "tcp"),
        "suffix": values.get("CONTAINER_SUFFIX", ""),
        "services": [],
    }


def load_deploy_state() -> Dict[str, Any]:
    if DEPLOY_STATE_FILE.exists():
        try:
            raw = json.loads(DEPLOY_STATE_FILE.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            return {}
        return raw if isinstance(raw, dict) else {}
    return _read_generated_info_state()


def desired_services_state() -> List[Dict[str, Any]]:
    return [_service_state(item) for item in config.SERVICES]


def classify_services(previous: Dict[str, Any]) -> Dict[str, List[str]]:
    previous_services = {
        str(item.get("alias")): item
        for item in previous.get("services", [])
        if isinstance(item, dict) and item.get("alias")
    }
    desired_services = {item["alias"]: item for item in desired_services_state()}

    kept: List[str] = []
    changed: List[str] = []
    added: List[str] = []
    removed: List[str] = []

    for alias, desired in desired_services.items():
        old = previous_services.get(alias)
        if old is None:
            added.append(alias)
        elif old == desired:
            kept.append(alias)
        else:
            changed.append(alias)

    for alias in previous_services:
        if alias not in desired_services:
            removed.append(alias)

    return {
        "kept": sorted(kept),
        "changed": sorted(changed),
        "added": sorted(added),
        "removed": sorted(removed),
    }


def previous_http_safe_aliases(previous: Dict[str, Any]) -> List[str]:
    aliases: List[str] = []
    for item in previous.get("services", []):
        if not isinstance(item, dict):
            continue
        if item.get("mode") == "http":
            safe = str(item.get("safe_alias") or safe_alias(str(item.get("alias", ""))))
            if safe:
                aliases.append(safe)
    return aliases


def write_deploy_state(ctx: DeployContext) -> None:
    DEPLOY_STATE_FILE.parent.mkdir(parents=True, exist_ok=True)
    data = {
        "version": 1,
        "config_file": str(config.CONFIG_FILE),
        "root_domain": ctx.root_domain,
        "email": ctx.email,
        "vps_public_ip": ctx.vps_public_ip,
        "host_iface": ctx.host_iface,
        "frp_version": ctx.frp_version,
        "status_app_version": ctx.status_app_version,
        "bind_port": ctx.bind_port,
        "dashboard_port": ctx.dashboard_port,
        "status_port": ctx.status_port,
        "token": ctx.token,
        "dashboard_user": ctx.dashboard_user,
        "dashboard_password": ctx.dashboard_password,
        "frpc_use_encryption": config.FRPC_USE_ENCRYPTION,
        "frpc_use_compression": config.FRPC_USE_COMPRESSION,
        "frpc_tcp_mux": config.FRPC_TCP_MUX,
        "frpc_protocol": config.FRPC_PROTOCOL,
        "suffix": ctx.suffix,
        "services": desired_services_state(),
    }
    DEPLOY_STATE_FILE.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
