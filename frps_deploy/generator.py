"""生成配置文件：frps.toml、frpc.toml、docker-compose、nginx、.env。"""
from __future__ import annotations

from frps_deploy import config
from frps_deploy.constants import (
    CERTBOT_CONF_DIR, CERTBOT_WWW_DIR, COMPOSE_FILE, FRPC_BASE_DIR,
    FRPC_COMPOSE_FILE, FRPC_TOML_FILE, FRPS_TOML_FILE, GENERATED_INFO_FILE,
    NGINX_CONF_DIR, STATUS_APP_DATA_DIR, STATUS_APP_DIR, STATUS_APP_ENV_FILE,
)
from frps_deploy.models import DeployContext
from frps_deploy.services import (
    all_remote_ports, dashboard_domain, exposed_http_remote_ports,
    http_remote_ports, http_services, local_ip, local_port, managed_domains,
    needs_tunnel, remote_port, status_domain, tcp_remote_ports, tcp_services,
    tunneled_services, upstream_host,
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
        f"serverAddr = {toml_str(ctx.vps_public_ip)}",
        f"serverPort = {ctx.bind_port}",
        f"loginFailExit = false",
        "",
        "[auth]",
        'method = "token"',
        f"token = {toml_str(ctx.token)}",
        "",
        "[transport.tls]",
        "enable = true",
    ]
    for item in tunneled_services():
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

    status_section = ""
    if config.STATUS_APP_ENABLED:
        status_bind = "0.0.0.0" if config.STATUS_APP_HTTP else "127.0.0.1"
        domains_str = ",".join(managed_domains(ctx.root_domain))
        status_section = f"""
  frps-status:
    image: lishi0105/frps-status:latest
    container_name: frps_status_{ctx.suffix}
    restart: unless-stopped
    depends_on:
      - frps
    ports:
      - "{status_bind}:{ctx.status_port}:8080"
    environment:
      LISTEN: "0.0.0.0:8080"
      DB_PATH: "/data/frps-status.sqlite"
      FRPS_HOST: "frps"
      FRPS_BIND_PORT: "{ctx.bind_port}"
      FRPS_DASHBOARD_PORT: "{ctx.dashboard_port}"
      FRPS_DASHBOARD_USER: "{ctx.dashboard_user}"
      FRPS_DASHBOARD_PASSWORD: "{ctx.dashboard_password}"
      STATUS_DOMAINS: "{domains_str}"
      STATUS_USER: "{ctx.status_user}"
      STATUS_PASSWORD: "{ctx.status_password}"
      CERT_DIR: "/etc/letsencrypt/live"
      POLL_SECONDS: "60"
      HOST_PUBLIC_IP: "{ctx.vps_public_ip}"
      HOST_IFACE: "{ctx.host_iface}"
      HOST_NET_STATS_DIR: "/host-net-stats"
    volumes:
      - ./frps-status/data:/data
      - ./certbot/conf:/etc/letsencrypt:ro
      - /sys/class/net/{ctx.host_iface}/statistics:/host-net-stats:ro
"""

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
{status_section}"""
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
    domains = managed_domains(ctx.root_domain)
    if not domains:
        return
    server_names = " ".join(domains)
    conf = f"""server {{
    listen 80;
    server_name {server_names};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

    location / {{
        return 200 "certbot challenge server is running\\n";
        add_header Content-Type text/plain;
    }}
}}
"""
    (NGINX_CONF_DIR / "00-http-challenge.conf").write_text(conf, encoding="utf-8")


def generate_https_nginx_confs(ctx: DeployContext) -> None:
    challenge_conf = NGINX_CONF_DIR / "00-http-challenge.conf"
    if challenge_conf.exists():
        challenge_conf.unlink()

    frps_domain = dashboard_domain(ctx.root_domain)
    frps_conf = f"""# frps dashboard
server {{
    listen 80;
    server_name {frps_domain};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

    location / {{
        return 301 https://$host$request_uri;
    }}
}}

server {{
    listen 443 ssl http2;
    server_name {frps_domain};

    ssl_certificate /etc/letsencrypt/live/{frps_domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{frps_domain}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    location / {{
        proxy_pass http://frps:{ctx.dashboard_port};

        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
    }}
}}
"""
    (NGINX_CONF_DIR / "frps-dashboard.conf").write_text(frps_conf, encoding="utf-8")

    if config.STATUS_APP_ENABLED:
        status_host = status_domain(ctx.root_domain)
        status_conf = f"""# frps status app
server {{
    listen 80;
    server_name {status_host};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

    location / {{
        return 301 https://$host$request_uri;
    }}
}}

server {{
    listen 443 ssl http2;
    server_name {status_host};

    ssl_certificate /etc/letsencrypt/live/{status_host}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/{status_host}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;

    client_max_body_size 0;

    location / {{
        proxy_pass http://frps-status:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;

        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }}
}}
"""
        (NGINX_CONF_DIR / "frps-status.conf").write_text(status_conf, encoding="utf-8")

    for item in http_services():
        alias  = str(item["alias"])
        domain = f"{alias}.{ctx.root_domain}"
        proxy_target = (
            f"http://frps:{remote_port(item)}"
            if needs_tunnel(item)
            else f"http://{upstream_host(item)}:{local_port(item)}"
        )
        comment = str(item.get("comment", alias))
        conf = f"""# {comment}
server {{
    listen 80;
    server_name {domain};

    location /.well-known/acme-challenge/ {{
        root /var/www/certbot;
    }}

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

    location / {{
        proxy_pass {proxy_target};

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


def remove_https_confs(previous_http_safe_aliases: list[str] | None = None) -> None:
    for name in ("frps-dashboard.conf", "frps-status.conf"):
        p = NGINX_CONF_DIR / name
        if p.exists():
            p.unlink()
    for alias in previous_http_safe_aliases or []:
        p = NGINX_CONF_DIR / f"{alias}.conf"
        if p.exists():
            p.unlink()
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
        f"VPS_PUBLIC_IP={ctx.vps_public_ip}",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"FRPS_TOKEN={ctx.token}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"FRPS_DASHBOARD_URL=https://{dashboard_domain(ctx.root_domain)}",
        f"CONTAINER_SUFFIX={ctx.suffix}",
        f"FRPS_DIR={BASE_DIR}",
        f"FRPC_DIR={_FRPC}",
        "",
        "# HTTP services",
    ]
    if config.STATUS_APP_ENABLED:
        lines[9:9] = [
            f"STATUS_PORT={ctx.status_port}",
            f"STATUS_LOCAL_URL=http://127.0.0.1:{ctx.status_port}",
            f"STATUS_HTTPS_URL=https://{status_domain(ctx.root_domain)}",
        ]
    for item in http_services():
        lines.append(
            f"HTTP_{item['alias'].upper()}=https://{item['alias']}.{ctx.root_domain} "
            f"remotePort={remote_port(item)} local={local_ip(item)}:{local_port(item)}"
        )
    lines += ["", "# TCP services"]
    for item in tcp_services():
        lines.append(
            f"TCP_{str(item['alias']).upper()}={ctx.vps_public_ip}:{remote_port(item)} "
            f"local={local_ip(item)}:{local_port(item)}"
        )
    GENERATED_INFO_FILE.write_text("\n".join(lines) + "\n", encoding="utf-8")


def write_status_app_env(ctx: DeployContext) -> None:
    if not config.STATUS_APP_ENABLED:
        return
    STATUS_APP_DIR.mkdir(parents=True, exist_ok=True)
    STATUS_APP_DATA_DIR.mkdir(parents=True, exist_ok=True)
    lines = [
        "LISTEN=0.0.0.0:8080",
        f"STATUS_APP_BIND={'0.0.0.0' if config.STATUS_APP_HTTP else '127.0.0.1'}",
        f"STATUS_APP_PORT={ctx.status_port}",
        "DB_PATH=/data/frps-status.sqlite",
        "FRPS_HOST=frps",
        f"FRPS_BIND_PORT={ctx.bind_port}",
        f"FRPS_DASHBOARD_PORT={ctx.dashboard_port}",
        f"FRPS_DASHBOARD_USER={ctx.dashboard_user}",
        f"FRPS_DASHBOARD_PASSWORD={ctx.dashboard_password}",
        f"STATUS_DOMAINS={','.join(managed_domains(ctx.root_domain))}",
        f"STATUS_USER={ctx.dashboard_user}",
        f"STATUS_PASSWORD={ctx.dashboard_password}",
        "CERT_DIR=/etc/letsencrypt/live",
        "POLL_SECONDS=60",
        f"HOST_PUBLIC_IP={ctx.vps_public_ip}",
        f"HOST_IFACE={ctx.host_iface}",
        "HOST_NET_STATS_DIR=/host-net-stats",
        f"HOST_NET_STATS_MOUNT=/sys/class/net/{ctx.host_iface}/statistics",
        f"FRPS_CERTBOT_CONF_DIR={CERTBOT_CONF_DIR}",
        "",
    ]
    STATUS_APP_ENV_FILE.write_text("\n".join(lines), encoding="utf-8")
