"""全局运行时状态与配置加载。"""
from __future__ import annotations

import json
from pathlib import Path
from typing import Any, Dict, List

from frps_deploy.console import print, eprint
from frps_deploy.constants import DEFAULT_CONFIG, DEFAULT_CONFIG_FILE, DEFAULT_SERVICES


class ConfigFileCreated(RuntimeError):
    pass


# ── 可被 main.py 修改的全局变量 ──────────────────────────────
CONFIG_FILE: Path = DEFAULT_CONFIG_FILE

# ── 运行时解析结果 ────────────────────────────────────────────
CONFIG: Dict[str, Any] = {}
SERVICES: List[Dict[str, Any]] = []
ROOT_DOMAIN = ""
CERT_EMAIL  = ""
VPS_PUBLIC_IP = ""
FRPS_SERVER_PORT   = 0
FRPS_TOKEN         = ""
FRPS_DASHBOARD_HTTP = False
FRPS_ENABLE_PROMETHEUS = True
FRPC_USE_ENCRYPTION = True
FRPC_USE_COMPRESSION = True
STATUS_APP_ENABLED   = True
STATUS_APP_PORT      = 0
STATUS_APP_HTTP      = False
STATUS_APP_USER      = "admin"
STATUS_APP_PASSWORD  = "admin123"


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
    print("请按需填写主域名（root_domain）、证书邮箱（cert_email）与服务列表（services）。")


def load_config_file() -> Dict[str, Any]:
    if not CONFIG_FILE.exists():
        write_default_config()
        raise ConfigFileCreated(str(CONFIG_FILE))

    try:
        raw = json.loads(CONFIG_FILE.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise ValueError(f"配置文件不是合法 JSON：{CONFIG_FILE}，{exc}") from exc

    if not isinstance(raw, dict):
        raise ValueError(f"配置文件顶层必须是 JSON 对象：{CONFIG_FILE}")

    data = clone_default_config()
    data.update(raw)
    return data


def _require_object(value: Any, key: str) -> Dict[str, Any]:
    if not isinstance(value, dict):
        raise ValueError(f"配置项 {key} 必须是 JSON 对象")
    return value


def _validate_runtime_config_shape(data: Dict[str, Any]) -> None:
    for key in ("root_domain", "domain", "cert_email", "certificate_email", "email", "vps_public_ip", "public_ip"):
        if key in data and data[key] is not None and not isinstance(data[key], str):
            raise ValueError(f"配置项 {key} 必须是字符串")

    _require_object(data.get("frps"), "frps")
    _require_object(data.get("status"), "status")
    if "frpc" in data:
        _require_object(data.get("frpc"), "frpc")

    services = data.get("services", DEFAULT_SERVICES)
    if not isinstance(services, list):
        raise ValueError("配置项 services 必须是 JSON 数组")
    for index, item in enumerate(services, start=1):
        if not isinstance(item, dict):
            raise ValueError(f"services[{index}] 必须是 JSON 对象")


def _bool_config(value: Any, default: bool = False) -> bool:
    if isinstance(value, bool):
        return value
    if value is None:
        return default
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "y", "on"}
    return bool(value)


def load_runtime_config() -> None:
    global CONFIG, SERVICES, ROOT_DOMAIN, CERT_EMAIL, VPS_PUBLIC_IP, FRPS_SERVER_PORT, FRPS_TOKEN, FRPS_DASHBOARD_HTTP, FRPS_ENABLE_PROMETHEUS, FRPC_USE_ENCRYPTION, FRPC_USE_COMPRESSION, STATUS_APP_ENABLED, STATUS_APP_PORT, STATUS_APP_HTTP, STATUS_APP_USER, STATUS_APP_PASSWORD

    CONFIG = load_config_file()
    _validate_runtime_config_shape(CONFIG)

    services = CONFIG.get("services", DEFAULT_SERVICES)

    SERVICES    = services
    ROOT_DOMAIN = str(CONFIG.get("root_domain") or CONFIG.get("domain") or "").strip().lower()
    CERT_EMAIL  = str(CONFIG.get("cert_email") or CONFIG.get("certificate_email") or CONFIG.get("email") or "").strip()
    VPS_PUBLIC_IP = str(CONFIG.get("vps_public_ip") or CONFIG.get("public_ip") or "").strip()

    frps_config = _require_object(CONFIG.get("frps"), "frps")
    FRPS_TOKEN = str(frps_config.get("token") or "").strip()

    raw_server_port = frps_config.get("server_port", 0)
    try:
        FRPS_SERVER_PORT = int(raw_server_port or 0)
    except (TypeError, ValueError) as exc:
        raise ValueError(f"frps.server_port 必须是整数：{raw_server_port!r}") from exc

    FRPS_DASHBOARD_HTTP = _bool_config(frps_config.get("dashboard_http", False), default=False)
    FRPS_ENABLE_PROMETHEUS = _bool_config(frps_config.get("enable_prometheus", True), default=True)

    frpc_config = _require_object(CONFIG.get("frpc", {}), "frpc")
    FRPC_USE_ENCRYPTION = _bool_config(frpc_config.get("use_encryption", True), default=True)
    FRPC_USE_COMPRESSION = _bool_config(frpc_config.get("use_compression", True), default=True)

    status_config = _require_object(CONFIG.get("status"), "status")

    STATUS_APP_ENABLED = _bool_config(status_config.get("enabled", True), default=True)
    raw_status_port = status_config.get("port", 0)
    try:
        STATUS_APP_PORT = int(raw_status_port or 0)
    except (TypeError, ValueError) as exc:
        raise ValueError(f"status.port 必须是整数：{raw_status_port!r}") from exc
    STATUS_APP_HTTP = _bool_config(status_config.get("http", False), default=False)
    STATUS_APP_USER     = str(status_config.get("user") or "admin").strip() or "admin"
    STATUS_APP_PASSWORD = str(status_config.get("password") or "admin123").strip() or "admin123"
