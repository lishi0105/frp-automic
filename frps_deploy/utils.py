"""通用工具：子进程、端口、随机数、toml 辅助。"""
from __future__ import annotations

import ipaddress
import json
import os
import random
import secrets
import shutil
import socket
import string
import subprocess
from pathlib import Path
from typing import List

from frps_deploy.console import print


def run(cmd: List[str], check: bool = True, cwd: Path | None = None) -> subprocess.CompletedProcess:
    print(f"\n$ {' '.join(cmd)}")
    return subprocess.run(cmd, check=check, cwd=str(cwd) if cwd else None, text=True)


def capture(cmd: List[str], check: bool = True) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, check=check, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)


def command_exists(name: str) -> bool:
    return shutil.which(name) is not None


def toml_str(value: str) -> str:
    return json.dumps(value, ensure_ascii=False)


def safe_alias(alias: str) -> str:
    allowed = string.ascii_letters + string.digits + "_-"
    return "".join(c if c in allowed else "_" for c in alias)


def random_letters(n: int = 16) -> str:
    return "".join(secrets.choice(string.ascii_letters) for _ in range(n))


def random_password(n: int = 16) -> str:
    chars = string.ascii_letters + string.digits + "!@#$%^&*()-_=+"
    while True:
        pwd = "".join(secrets.choice(chars) for _ in range(n))
        if (
            any(c.islower() for c in pwd)
            and any(c.isupper() for c in pwd)
            and any(c.isdigit() for c in pwd)
            and any(c in "!@#$%^&*()-_=+" for c in pwd)
        ):
            return pwd


def is_port_free(port: int, host: str = "127.0.0.1") -> bool:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.settimeout(0.3)
        return s.connect_ex((host, port)) != 0


def random_free_port(start: int = 20000, end: int = 60000) -> int:
    for _ in range(200):
        port = random.randint(start, end)
        if is_port_free(port):
            return port
    raise RuntimeError("无法找到可用随机端口")


def random_free_port_excluding(used: set, start: int = 20000, end: int = 60000) -> int:
    for _ in range(200):
        port = random_free_port(start, end)
        if port not in used:
            used.add(port)
            return port
    raise RuntimeError("无法找到不冲突的可用随机端口")


def validate_ipv4(ip: str) -> str:
    try:
        parsed = ipaddress.ip_address(ip)
    except ValueError as exc:
        raise ValueError(f"不是合法 IP 地址：{ip}") from exc
    if parsed.version != 4:
        raise ValueError(f"当前脚本只自动创建 A 记录，请输入 IPv4 地址：{ip}")
    return str(parsed)
