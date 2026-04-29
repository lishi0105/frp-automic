# 本地启动
```bash
cd /home/lishi/frp-automic/frps-status-app
docker network create frps_default
docker compose up -d --build

```
# 本地指定账号和密码
```bash
FRPS_DASHBOARD_USER=admin FRPS_DASHBOARD_PASSWORD=123456 STATUS_APP_BIND=0.0.0.0 STATUS_APP_PORT=28080 docker compose up -d --build
```