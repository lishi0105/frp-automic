"""清理生成的容器、镜像和文件夹。"""
from __future__ import annotations

import shutil
import subprocess
from pathlib import Path

from frps_deploy.console import eprint, print
from frps_deploy.constants import (
    BASE_DIR, FRPC_BASE_DIR,
    STATUS_APP_DATA_DIR, STATUS_APP_ENV_FILE,
)


def _compose_down(cwd: Path) -> None:
    if not cwd.exists():
        return
    compose_file = cwd / "docker-compose.yml"
    if not compose_file.exists():
        return
    cmd = ["docker", "compose", "down"]
    print(f"\n$ {' '.join(cmd)}  (cwd={cwd})")
    subprocess.run(cmd, cwd=str(cwd), check=False)


def _fix_permissions(path: Path) -> None:
    """通过 alpine 容器将目录权限修改为 777，以便后续以普通用户身份删除。"""
    if not path.exists():
        return
    cmd = [
        "docker", "run", "--rm",
        "-v", f"{path}:/target",
        "alpine",
        "chmod", "-R", "777", "/target",
    ]
    print(f"\n$ {' '.join(cmd)}")
    result = subprocess.run(cmd, check=False)
    if result.returncode != 0:
        print(f"警告：chmod 容器对 {path} 返回非零退出码，将尝试直接删除。")


def _remove_dir(path: Path) -> None:
    if not path.exists():
        return
    print(f"删除目录：{path}")
    try:
        shutil.rmtree(path)
    except PermissionError as exc:
        eprint(f"删除失败（权限不足）：{path}  原因：{exc}")
        eprint("提示：请尝试手动执行 sudo rm -rf 或重新运行 --clean 确保 docker 可访问。")


def _remove_file(path: Path) -> None:
    if not path.exists():
        return
    print(f"删除文件：{path}")
    try:
        path.unlink()
    except PermissionError as exc:
        eprint(f"删除失败（权限不足）：{path}  原因：{exc}")


def clean_all() -> None:
    print("=== 清理部署资源 ===")

    # 1. 停止并删除容器
    _compose_down(BASE_DIR)
    _compose_down(FRPC_BASE_DIR)

    # 2. 修改 root 可能产生的文件权限（certbot 会以 root 写入 conf/）
    for d in (BASE_DIR, STATUS_APP_DATA_DIR):
        _fix_permissions(d)

    # 3. 删除生成的目录和文件
    _remove_dir(BASE_DIR)
    _remove_dir(FRPC_BASE_DIR)
    _remove_dir(STATUS_APP_DATA_DIR)
    _remove_file(STATUS_APP_ENV_FILE)

    print("\n清理完成。")


def stop_all() -> None:
    print("=== 停止部署服务 ===")
    _compose_down(BASE_DIR)
    _compose_down(FRPC_BASE_DIR)
    print("\n已停止所有已生成的 Docker Compose 服务。")
