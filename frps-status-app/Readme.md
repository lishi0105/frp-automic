# 本地启动
```bash
cd /home/lishi/frp-automic/frps-status-app
docker network create frps_default
docker compose up -d --build

```
# 本地编译
```bash
docker compose build --no-cache --pull
```