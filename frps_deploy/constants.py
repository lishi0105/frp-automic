"""路径常量与默认配置。"""
from __future__ import annotations

from pathlib import Path
from typing import Any, Dict, List

# 项目根目录（frps_deploy/ 的上级）
SCRIPT_DIR = Path(__file__).resolve().parent.parent

DEFAULT_FRP_VERSION = "v0.68.1"

DEFAULT_CONFIG_FILE = SCRIPT_DIR / "frps-config.json"

BASE_DIR          = SCRIPT_DIR / "frps"
NGINX_CONF_DIR    = BASE_DIR / "nginx" / "conf.d"
CERTBOT_CONF_DIR  = BASE_DIR / "certbot" / "conf"
CERTBOT_WWW_DIR   = BASE_DIR / "certbot" / "www"
GENERATED_INFO_FILE = BASE_DIR / ".env.generated.txt"
COMPOSE_FILE      = BASE_DIR / "docker-compose.yml"
FRPS_TOML_FILE    = BASE_DIR / "frps.toml"

FRPC_BASE_DIR     = SCRIPT_DIR / "frpc"
FRPC_COMPOSE_FILE = FRPC_BASE_DIR / "docker-compose.yml"
FRPC_TOML_FILE    = FRPC_BASE_DIR / "frpc.toml"

STATUS_APP_DIR      = SCRIPT_DIR / "frps-status-app"
STATUS_APP_ENV_FILE = STATUS_APP_DIR / ".env"
STATUS_APP_DATA_DIR = STATUS_APP_DIR / "data"

DEFAULT_SERVICES: List[Dict[str, Any]] = [
    {"alias": "emby",      "comment": "Emby 媒体服务", "mode": "http", "port": 7096, "local_port": 7096, "local_ip": "127.0.0.1"},
    {"alias": "gitlab",    "comment": "GitLab 服务",   "mode": "http", "port": 5080},
    {"alias": "speedtest", "comment": "Speedtest 服务","mode": "http", "port": 3010},
    {"alias": "gitlab_ssh","comment": "GitLab SSH",    "mode": "tcp",  "port": 5022, "local_port": 22, "local_ip": "127.0.0.1"},
]

DEFAULT_CONFIG: Dict[str, Any] = {
    "root_domain": "",
    "cert_email": "",
    "frps_server_port": 0,
    "frps_token": "",
    "services": DEFAULT_SERVICES,
}
