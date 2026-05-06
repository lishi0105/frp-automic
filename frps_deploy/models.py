"""数据模型。"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class DeployContext:
    root_domain: str
    email: str
    vps_public_ip: str
    host_iface: str
    frp_version: str
    bind_port: int
    dashboard_port: int
    status_port: int
    token: str
    dashboard_user: str
    dashboard_password: str
    status_user: str
    status_password: str
    suffix: str
