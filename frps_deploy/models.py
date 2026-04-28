"""数据模型。"""
from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class DeployContext:
    root_domain: str
    email: str
    frp_version: str
    bind_port: int
    dashboard_port: int
    status_port: int
    token: str
    dashboard_user: str
    dashboard_password: str
    suffix: str
