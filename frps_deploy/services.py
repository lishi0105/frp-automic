"""服务配置解析与校验。"""
from __future__ import annotations

from typing import Any, Dict, List

from frps_deploy import config


def mode_of(item: Dict[str, Any]) -> str:
    return str(item.get("mode", "http")).strip().lower()


def remote_port(item: Dict[str, Any]) -> int:
    return int(item["port"])


def local_port(item: Dict[str, Any]) -> int:
    return int(item.get("local_port", item["port"]))


def local_ip(item: Dict[str, Any]) -> str:
    return str(item.get("local_ip", "127.0.0.1"))


def http_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if mode_of(s) == "http"]


def tcp_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if mode_of(s) == "tcp"]


def all_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in config.SERVICES})


def http_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in http_services()})


def tcp_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in tcp_services()})


def http_domains(root_domain: str) -> List[str]:
    return [f"{s['alias']}.{root_domain}" for s in http_services()]


def validate_services() -> None:
    aliases: set = set()
    ports: set = set()

    for item in config.SERVICES:
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
            for ch in alias:
                if not (ch.isalnum() or ch == "-"):
                    raise ValueError(
                        f"HTTP 服务 alias 需要能作为子域名前缀，当前非法：{alias}。"
                        "建议使用 emby/test/panel 这类字母数字短横线。"
                    )
