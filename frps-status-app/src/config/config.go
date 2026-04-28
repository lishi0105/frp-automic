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
	Domains           []string
	StatusUser        string
	StatusPassword    string
	PollInterval      time.Duration
}

func Load() Config {
	return Config{
		Listen:            env("LISTEN", "127.0.0.1:28080"),
		DBPath:            env("DB_PATH", "/data/frps-status.sqlite"),
		FRPSHost:          env("FRPS_HOST", "127.0.0.1"),
		FRPSBindPort:      envInt("FRPS_BIND_PORT", 7000),
		FRPSDashboardPort: envInt("FRPS_DASHBOARD_PORT", 7500),
		FRPSDashboardUser: env("FRPS_DASHBOARD_USER", ""),
		FRPSDashboardPass: env("FRPS_DASHBOARD_PASSWORD", ""),
		CertDir:           env("CERT_DIR", "/etc/letsencrypt/live"),
		Domains:           SplitCSV(os.Getenv("STATUS_DOMAINS")),
		StatusUser:        env("STATUS_USER", ""),
		StatusPassword:    env("STATUS_PASSWORD", ""),
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
