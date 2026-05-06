package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Listen            string
	DBPath            string
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
}

func Load() Config {
	frpsUser := env("FRPS_DASHBOARD_USER", "")
	frpsPass := env("FRPS_DASHBOARD_PASSWORD", "")
	return Config{
		Listen:            env("LISTEN", "127.0.0.1:28080"),
		DBPath:            env("DB_PATH", "/data/frps-status.sqlite"),
		FRPSHost:          env("FRPS_HOST", "127.0.0.1"),
		FRPSBindPort:      envInt("FRPS_BIND_PORT", 7000),
		FRPSDashboardPort: envInt("FRPS_DASHBOARD_PORT", 7500),
		FRPSDashboardUser: frpsUser,
		FRPSDashboardPass: frpsPass,
		CertDir:           env("CERT_DIR", "/etc/letsencrypt/live"),
		HostPublicIP:      env("HOST_PUBLIC_IP", ""),
		HostIface:         env("HOST_IFACE", ""),
		HostNetStatsDir:   env("HOST_NET_STATS_DIR", "/host-net-stats"),
		Domains:           SplitCSV(os.Getenv("STATUS_DOMAINS")),
		StatusUser:        env("STATUS_USER", frpsUser),
		StatusPassword:    env("STATUS_PASSWORD", frpsPass),
		LogDir:            env("LOG_DIR", ""),
		PollInterval:      time.Duration(envInt("POLL_SECONDS", 60)) * time.Second,
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
