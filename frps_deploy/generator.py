"""生成配置文件：frps.toml、frpc.toml、docker-compose、nginx、.env。"""
from __future__ import annotations

from frps_deploy import config
from frps_deploy.console import print
from frps_deploy.constants import (
    CERTBOT_CONF_DIR, CERTBOT_WWW_DIR, COMPOSE_FILE, FRPC_BASE_DIR,
    FRPC_COMPOSE_FILE, FRPC_TOML_FILE, FRPS_TOML_FILE, GENERATED_INFO_FILE,
    NGINX_CONF_DIR, STATUS_APP_DATA_DIR, STATUS_APP_DIR, STATUS_APP_ENV_FILE,
)
from frps_deploy.models import DeployContext
from frps_deploy.services import (
    all_remote_ports, exposed_http_remote_ports, http_domains, http_remote_ports, http_services,
    local_ip, local_port, remote_port, tcp_remote_ports, tcp_services,
)
from frps_deploy.utils import safe_alias, toml_str


# re-export safe_alias so generator.py's callers don't need to import utils directly
__all__ = [
    "ensure_dirs", "generate_frps_toml", "generate_frpc_toml",
    "generate_frps_compose", "generate_frpc_compose",
    "generate_http_challenge_conf", "generate_https_nginx_confs",
    "remove_https_confs", "write_generated_info", "write_status_app_env",
]


def ensure_dirs() -> None:
    NGINX_CONF_DIR.mkdir(parents=True, exist_ok=True)
    CERTBOT_CONF_DIR.mkdir(parents=True, exist_ok=True)
    CERTBOT_WWW_DIR.mkdir(parents=True, exist_ok=True)
    if config.STATUS_APP_ENABLED:
        STATUS_APP_DATA_DIR.mkdir(parents=True, exist_ok=True)
    FRPC_BASE_DIR.mkdir(parents=True, exist_ok=True)


def status_nginx_location(ctx: DeployContext, forwarded_proto: str) -> str:
    if not config.STATUS_APP_ENABLED:
        return ""
    return f"""
    location = /_frps-status {{
        return 301 /_frps-status/;
    }}

    location /_frps-status/ {{
        proxy_pass http://host.docker.internal:{ctx.status_port}/;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto {forwarded_proto};
    }}
"""


def generate_frps_toml(ctx: DeployContext) -> None:
    lines = [
        'bindAddr = "0.0.0.0"',
        f"bindPort = {ctx.bind_port}",
        "",
        "[auth]",
        'method = "token"',
        f"token = {toml_str(ctx.token)}",
        "",
        "[transport.tls]",
        "force = true",
        "",
        "[webServer]",
        'addr = "0.0.0.0"',
        f"port = {ctx.dashboard_port}",
        f"user = {toml_str(ctx.dashboard_user)}",
        f"password = {toml_str(ctx.dashboard_password)}",
        "",
    ]
    for port in all_remote_ports():
        lines += ["[[allowPorts]]", f"single = {port}", ""]
    FRPS_TOML_FILE.write_text("\n".join(lines), encoding="utf-8")


def generate_frpc_toml(ctx: DeployContext) -> None:
    lines = [
        'serverAddr = "<你的VPS公网IP>"',
        f"serverPort = {ctx.bind_port}",
        "",
        "[auth]",
        'method = "token"',
        f"token = {toml_str(ctx.token)}",
        "",
        "[transport.tls]",
        "enable = true",
    ]
    for item in config.SERVICES:
        lines += [
            "",
            f"# {item.get('comment', item['alias'])}",
            "[[proxies]]",
            f"name = {toml_str(str(item['alias']))}",
            'type = "tcp"',
            f"localIP = {toml_str(local_ip(item))}",
            f"localPort = {local_port(item)}",
            f"remotePort = {remote_port(item)}",
        ]
    FRPC_TOML_FILE.write_text("\n".join(lines) + "\n", encoding="utf-8")


def generate_frps_compose(ctx: DeployContext) -> None:
    dashboard_bind = "" if config.FRPS_DASHBOARD_HTTP else "127.0.0.1:"
    port_lines = [
        f'      - "{ctx.bind_port}:{ctx.bind_port}/tcp"',
        f'      - "{dashboard_bind}{ctx.dashboard_port}:{ctx.dashboard_port}/tcp"',
    ] + [f'      - "{p}:{p}/tcp"' for p in tcp_remote_ports()]

    for p in exposed_http_remote_ports():
        port_lines.append(f'      - "{p}:{p}/tcp"')

    expose_section = ""
    if http_remote_ports():
        expose_lines = "\n".join(f'      - "{p}"' for p in http_remote_ports())
        expose_section = f"\n    expose:\n{expose_lines}"

    compose = f"""services:
  frps:
    image: fatedier/frps:{ctx.frp_version}
    container_name: frps_{ctx.suffix}
    restart: unless-stopped
    volumes:
      - ./frps.toml:/etc/frp/frps.toml:ro
    command: ["-c", "/etc/frp/frps.toml"]
    ports:
{chr(10).join(port_lines)}
{expose_section}

  nginx:
    image: nginx:1.27-alpine
    container_name: nginx_{ctx.suffix}
    restart: unless-stopped
    depends_on:
      - frps
    extra_hosts:
      - "host.docker.internal:host-gateway"
    ports:
      - "80:80/tcp"
      - "443:443/tcp"
    volumes:
      - ./nginx/conf.d:/etc/nginx/conf.d:ro
      - ./certbot/www:/var/www/certbot:ro
      - ./certbot/conf:/etc/letsencrypt:ro
    command: >
      /bin/sh -c "while :; do sleep 6h; nginx -s reload || true; done &
      nginx -g 'daemon off;'"

  certbot:
    image: certbot/certbot:latest
    container_name: certbot_{ctx.suffix}
    restart: unless-stopped
    volumes:
      - ./certbot/www:/var/www/certbot
      - ./certbot/conf:/etc/letsencrypt
    entrypoint: >
      /bin/sh -c "trap exit TERM;
      while :; do
        certbot renew --webroot -w /var/www/certbot --quiet;
        sleep 12h & wait $${{!}};
      done"
"""
    COMPOSE_FILE.write_text(compose, encoding="utf-8")


def generate_frpc_compose(ctx: DeployContext) -> None:
    compose = f"""services:
  frpc:
    image: fatedier/frpc:{ctx.frp_version}
    container_name: frpc_{ctx.suffix}
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./frpc.toml:/etc/frp/frpc.toml:ro
    command: ["-c", "/etc/frp/frpc.toml"]
"""
    FRPC_COMPOSE_FILE.write_text(compose, encoding="utf-8")


def generate_http_challenge_conf(ctx: DeployContext) -> None:
    domains = http_domains(ctx.root_domain)
    if not domains:
        return
    server_names = " ".join(domains)
    conf = f"""server {{
    listen 80;
    server_name {server_names};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

{status_nginx_location(ctx, "$scheme")}

    location / {{
        return 200 "certbot challenge server is running\\n";
        add_header Content-Type text/plain;
    }}
}}
"""
    (NGINX_CONF_DIR / "00-http-challenge.conf").write_text(conf, encoding="utf-8")


def generate_https_nginx_confs(ctx: DeployContext) -> None:
    for item in http_services():
        alias  = str(item["alias"])
        domain = f"{alias}.{ctx.root_domain}"
        port   = remote_port(item)
        comment = str(item.get("comment", alias))
        conf = f"""# {comment}
server {{
    listen 80;
    server_name {domain};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

{status_nginx_location(ctx, "$scheme")}

    location / {{
        return 301 https://$host$request_uri;
    }}
}}

server {{
    listen 443 ssl http2;
    server_name {domain};

    ssl_certificate /etc/letsencrypt/live/{domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{domain}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    client_max_body_size 0;

{status_nginx_location(ctx, "https")}

    location / {{
        proxy_pass http://frps:{port};

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";

        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }}
}}
"""
        (NGINX_CONF_DIR / f"{safe_alias(alias)}.conf").write_text(conf, encoding="utf-8")


def remove_https_confs() -> None:
    for item in http_services():
        p = NGINX_CONF_DIR / f"{safe_alias(str(item['alias']))}.conf"
        if p.exists():
            p.unlink()


def write_generated_info(ctx: DeployContext) -> None:
    from frps_deploy.constants import BASE_DIR, FRPC_BASE_DIR as _FRPC
    lines = [
        f"ROOT_DOMAIN={ctx.root_domain}",
        f"EMAIL={ctx.email}",
        f"FRP_VERSION={ctx.frp_version}",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"FRPS_TOKEN={ctx.token}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"CONTAINER_SUFFIX={ctx.suffix}",
        f"FRPS_DIR={BASE_DIR}",
        f"FRPC_DIR={_FRPC}",
        "",
        "# HTTP services",
    ]
    if config.STATUS_APP_ENABLED:
        lines[5:5] = [
            f"STATUS_PORT={ctx.status_port}",
            f"STATUS_LOCAL_URL=http://127.0.0.1:{ctx.status_port}",
        ]
    for item in http_services():
        lines.append(
            f"HTTP_{item['alias'].upper()}=https://{item['alias']}.{ctx.root_domain} "
            f"remotePort={remote_port(item)} local={local_ip(item)}:{local_port(item)}"
        )
    lines += ["", "# TCP services"]
    for item in tcp_services():
        lines.append(
            f"TCP_{str(item['alias']).upper()}=<VPS_IP>:{remote_port(item)} "
            f"local={local_ip(item)}:{local_port(item)}"
        )
    GENERATED_INFO_FILE.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_status_app_env(ctx: DeployContext) -> None:
    if not config.STATUS_APP_ENABLED:
        return
    STATUS_APP_DIR.mkdir(parents=True, exist_ok=True)
    STATUS_APP_DATA_DIR.mkdir(parents=True, exist_ok=True)
    lines = [
        f"LISTEN={'0.0.0.0' if config.STATUS_APP_HTTP else '127.0.0.1'}:{ctx.status_port}",
        "DB_PATH=/data/frps-status.sqlite",
        "FRPS_HOST=127.0.0.1",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"FRPS_DASHBOARD_USER={ctx.dashboard_user}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"STATUS_DOMAINS={','.join(http_domains(ctx.root_domain))}",
        f"STATUS_USER={ctx.dashboard_user}",
        f"STATUS_PASSWORD={ctx.dashboard_password}",
        "CERT_DIR=/etc/letsencrypt/live",
        "POLL_SECONDS=60",
        f"FRPS_CERTBOT_CONF_DIR={CERTBOT_CONF_DIR}",
        "",
    ]
    STATUS_APP_ENV_FILE.write_text("\n".join(lines), encoding="utf-8")
