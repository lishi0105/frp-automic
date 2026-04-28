"""彩色时间戳输出、eprint、prompt_input。"""
from __future__ import annotations

import builtins
import sys
import time
from typing import Any, Optional

COLOR_RESET  = "\033[0m"
COLOR_DIM    = "\033[2m"
COLOR_RED    = "\033[31m"
COLOR_GREEN  = "\033[32m"
COLOR_YELLOW = "\033[33m"
COLOR_BLUE   = "\033[34m"
COLOR_CYAN   = "\033[36m"


def now_text() -> str:
    return time.strftime("%Y-%m-%d %H:%M:%S")


def _pick_color(text: str, file: Any) -> str:
    if file is sys.stderr:
        return COLOR_RED
    s = text.strip()
    if s.startswith("$ "):
        return COLOR_BLUE
    if "错误" in s or "失败" in s:
        return COLOR_RED
    if "警告" in s:
        return COLOR_YELLOW
    if "完成" in s or "通过" in s or "已创建" in s or "已更新" in s:
        return COLOR_GREEN
    return COLOR_CYAN


def print(
    *args: Any,
    sep: str = " ",
    end: str = "\n",
    file: Any = None,
    flush: bool = False,
    color: Optional[str] = None,
) -> None:
    target = file or sys.stdout
    message = sep.join(str(a) for a in args)
    if message == "":
        builtins.print("", end=end, file=target, flush=flush)
        return
    chosen = color or _pick_color(message, target)
    prefix = f"{COLOR_DIM}[{now_text()}]{COLOR_RESET} "
    lines = message.splitlines() or [""]
    for i, line in enumerate(lines):
        cur_end = end if i == len(lines) - 1 else "\n"
        builtins.print(f"{prefix}{chosen}{line}{COLOR_RESET}", end=cur_end, file=target, flush=flush)


def eprint(*args: Any) -> None:
    print(*args, file=sys.stderr)


def prompt_input(message: str) -> str:
    print(message, end="", color=COLOR_YELLOW)
    return builtins.input()
