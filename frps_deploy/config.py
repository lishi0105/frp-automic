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
FRPS_SERVER_PORT = 0
FRPS_TOKEN = ""


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
    print("请按需填写 root_domain、cert_email 和 services。")


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
    global CONFIG, SERVICES, ROOT_DOMAIN, CERT_EMAIL, FRPS_SERVER_PORT, FRPS_TOKEN

    CONFIG = load_config_file()

    services = CONFIG.get("services", DEFAULT_SERVICES)
    if not isinstance(services, list):
        raise ValueError("配置项 services 必须是数组")

    SERVICES    = services
    ROOT_DOMAIN = str(CONFIG.get("root_domain") or CONFIG.get("domain") or "").strip().lower()
    CERT_EMAIL  = str(CONFIG.get("cert_email") or CONFIG.get("certificate_email") or CONFIG.get("email") or "").strip()
    FRPS_TOKEN  = str(CONFIG.get("frps_token") or CONFIG.get("token") or "").strip()

    raw_server_port = CONFIG.get("frps_server_port", CONFIG.get("server_port", CONFIG.get("bind_port", 0)))
    try:
        FRPS_SERVER_PORT = int(raw_server_port or 0)
    except (TypeError, ValueError) as exc:
        raise ValueError(f"frps_server_port 必须是整数：{raw_server_port!r}") from exc
