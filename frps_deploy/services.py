"""服务配置解析与校验。"""
from __future__ import annotations

from typing import Any, Dict, List

from frps_deploy import config


def mode_of(item: Dict[str, Any]) -> str:
    return str(item.get("mode", "http")).strip().lower()


def bool_value(value: Any, default: bool = False) -> bool:
    if isinstance(value, bool):
        return value
    if value is None:
        return default
    if isinstance(value, str):
        return value.strip().lower() in {"1", "true", "yes", "y", "on"}
    return bool(value)


def needs_tunnel(item: Dict[str, Any]) -> bool:
    return bool_value(item.get("tunnel", True), default=True)


def remote_port(item: Dict[str, Any]) -> int:
    return int(item["port"])


def local_port(item: Dict[str, Any]) -> int:
    return int(item.get("local_port", item["port"]))


def local_ip(item: Dict[str, Any]) -> str:
    return str(item.get("local_ip", "127.0.0.1"))


def expose_http_port(item: Dict[str, Any]) -> bool:
    return bool_value(item.get("expose_http_port", False), default=False)


def iperf_test_enabled(item: Dict[str, Any]) -> bool:
    return needs_tunnel(item) and bool_value(item.get("iperf_test", False), default=False)


def iperf_local_port(item: Dict[str, Any]) -> int:
    return int(item.get("iperf_local_port", 5201))


def iperf_remote_port(item: Dict[str, Any]) -> int:
    return int(item["iperf_port"])


def iperf_proxy_name(item: Dict[str, Any]) -> str:
    return f"{item['alias']}_iperf3"


def upstream_host(item: Dict[str, Any]) -> str:
    host = local_ip(item).strip() or "127.0.0.1"
    if host in {"127.0.0.1", "localhost"}:
        return "host.docker.internal"
    return host


def tunneled_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if needs_tunnel(s)]


def http_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if mode_of(s) == "http"]


def tcp_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if mode_of(s) == "tcp"]


def all_remote_ports() -> List[int]:
    ports = {remote_port(s) for s in tunneled_services()}
    ports.update(iperf_remote_port(s) for s in iperf_test_services() if "iperf_port" in s)
    return sorted(ports)


def iperf_test_services() -> List[Dict[str, Any]]:
    return [s for s in config.SERVICES if iperf_test_enabled(s)]


def http_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in http_services() if needs_tunnel(s)})


def exposed_http_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in http_services() if needs_tunnel(s) and expose_http_port(s)})


def tcp_remote_ports() -> List[int]:
    return sorted({remote_port(s) for s in tcp_services() if needs_tunnel(s)})


def force_all_services_tcp() -> None:
    for item in config.SERVICES:
        if needs_tunnel(item):
            item["mode"] = "tcp"


def http_domains(root_domain: str) -> List[str]:
    return [f"{s['alias']}.{root_domain}" for s in http_services()]


def dashboard_domain(root_domain: str) -> str:
    return f"frps.{root_domain}"


def status_domain(root_domain: str) -> str:
    return f"status.{root_domain}"


def managed_domains(root_domain: str) -> List[str]:
    domains = http_domains(root_domain) + [dashboard_domain(root_domain)]
    if config.STATUS_APP_ENABLED:
        domains.append(status_domain(root_domain))
    return domains


def validate_services() -> None:
    aliases: set = set()
    ports: set = set()

    for index, item in enumerate(config.SERVICES, start=1):
        alias = str(item.get("alias", "")).strip()
        if not alias:
            raise ValueError(f"services[{index}] 缺少服务别名（alias）或别名为空")
        if alias in aliases:
            raise ValueError(f"各服务的别名（alias）重复：{alias}")
        if alias in {"frps", "status"}:
            raise ValueError(f"服务别名（alias）不能使用保留子域名：{alias}")
        aliases.add(alias)

        mode = mode_of(item)
        if mode not in {"http", "tcp"}:
            raise ValueError(f"服务 {alias} 的模式（mode）非法：{mode}，只支持 http/tcp")

        if "port" not in item:
            raise ValueError(f"服务 {alias} 缺少端口（port）")

        try:
            rp = remote_port(item)
        except (TypeError, ValueError) as exc:
            raise ValueError(f"服务 {alias} 的端口（port）必须是整数：{item.get('port')!r}") from exc
        try:
            lp = local_port(item)
        except (TypeError, ValueError) as exc:
            raise ValueError(f"服务 {alias} 的本地端口（local_port）必须是整数：{item.get('local_port')!r}") from exc
        if not (1000 <= rp <= 65535):
            raise ValueError(f"服务 {alias} 的端口（port）非法：{rp}")
        if not (1000 <= lp <= 65535):
            raise ValueError(f"服务 {alias} 的本地端口（local_port）非法：{lp}")
        if rp in ports:
            raise ValueError(f"各服务的远端端口（port）重复：{rp}")
        ports.add(rp)

        if iperf_test_enabled(item):
            try:
                ilp = iperf_local_port(item)
            except (TypeError, ValueError) as exc:
                raise ValueError(f"服务 {alias} 的测速本地端口（iperf_local_port）必须是整数：{item.get('iperf_local_port')!r}") from exc
            if not (1000 <= ilp <= 65535):
                raise ValueError(f"服务 {alias} 的测速本地端口（iperf_local_port）非法：{ilp}")
            if "iperf_port" in item and item.get("iperf_port") not in (None, ""):
                try:
                    irp = iperf_remote_port(item)
                except (TypeError, ValueError) as exc:
                    raise ValueError(f"服务 {alias} 的测速远端端口（iperf_port）必须是整数：{item.get('iperf_port')!r}") from exc
                if not (1000 <= irp <= 65535):
                    raise ValueError(f"服务 {alias} 的测速远端端口（iperf_port）非法：{irp}")
                if irp in ports:
                    raise ValueError(f"测速远端端口（iperf_port）与已使用端口冲突：{irp}")
                ports.add(irp)

        tunnel = item.get("tunnel", True)
        if not isinstance(tunnel, (bool, str, int)):
            raise ValueError(f"服务 {alias} 的隧道开关（tunnel）必须是布尔值")

        if mode == "http":
            for ch in alias:
                if not (ch.isalnum() or ch == "-"):
                    raise ValueError(
                        f"HTTP 服务的别名（alias）须能作为子域名前缀，当前非法：{alias}。"
                        "建议使用 emby、test、panel 这类仅含字母、数字与短横线的名称。"
                    )
