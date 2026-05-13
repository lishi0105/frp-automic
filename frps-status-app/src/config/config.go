package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen string
	DBPath string
	// ProcRoot 为空时使用 /proc；可设为挂载的宿主机 proc（如 /host/proc），便于容器内读取 loadavg/meminfo。
	ProcRoot          string
	FRPSHost          string
	FRPSBindPort      int
	FRPSDashboardPort int
	FRPSDashboardUser string
	FRPSDashboardPass string
	CertDir           string
	HostPublicIP      string
	HostIface         string
	HostNetStatsDir   string
	Domains           []string
	StatusUser        string
	StatusPassword    string
	LogDir            string
	PollInterval      time.Duration
	SpeedtestTargets  []SpeedtestTarget
}

type SpeedtestTarget struct {
	Name string `json:"name"`
	Host string `json:"host"`
	Port int    `json:"port"`
}

// 数据库路径固定（不由环境变量配置）。
const DBPathFixed = "/data/frps-status.sqlite"

// 日志目录固定为与数据、配置同级的 /logs；可通过 LOG_DIR 覆盖（一般留空即可）。
const LogDirFixed = "/logs"

// 容器内证书 live 目录固定（与 compose 将宿主机 certbot conf 挂载到 /etc/letsencrypt 的布局一致）。
const CertDirFixed = "/etc/letsencrypt/live"

// 状态面板 Web 登录固定（不由环境变量配置）；首次启动会写入 SQLite 用户表。
const StatusUserFixed = "admin"
const StatusPasswordFixed = "admin123"

func Load() Config {
	frpsUser := env("FRPS_DASHBOARD_USER", "")
	frpsPass := env("FRPS_DASHBOARD_PASSWORD", "")
	return Config{
		Listen:            env("LISTEN", "127.0.0.1:28080"),
		DBPath:            DBPathFixed,
		ProcRoot:          env("STATUS_PROC_ROOT", ""),
		FRPSHost:          env("FRPS_HOST", "127.0.0.1"),
		FRPSBindPort:      envInt("FRPS_BIND_PORT", 7000),
		FRPSDashboardPort: envInt("FRPS_DASHBOARD_PORT", 7500),
		FRPSDashboardUser: frpsUser,
		FRPSDashboardPass: frpsPass,
		CertDir:           CertDirFixed,
		HostPublicIP:      env("HOST_PUBLIC_IP", ""),
		HostIface:         env("HOST_IFACE", ""),
		HostNetStatsDir:   env("HOST_NET_STATS_DIR", "/host-net-stats"),
		Domains:           SplitCSV(os.Getenv("STATUS_DOMAINS")),
		StatusUser:        StatusUserFixed,
		StatusPassword:    StatusPasswordFixed,
		LogDir:            env("LOG_DIR", LogDirFixed),
		PollInterval:      time.Duration(envInt("POLL_SECONDS", 60)) * time.Second,
		SpeedtestTargets:  ParseSpeedtestTargets(os.Getenv("SPEEDTEST_TARGETS")),
	}
}

func SplitCSV(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func ParseSpeedtestTargets(v string) []SpeedtestTarget {
	var out []SpeedtestTarget
	for _, part := range strings.Split(v, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, addr, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		host, portText, ok := strings.Cut(strings.TrimSpace(addr), ":")
		if !ok {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(portText))
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		name = strings.TrimSpace(name)
		host = strings.TrimSpace(host)
		if name == "" || host == "" {
			continue
		}
		out = append(out, SpeedtestTarget{Name: name, Host: host, Port: port})
	}
	return out
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil {
		return fallback
	}
	return n
}
