package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"frps-status-app.local/status/src/appsettings"
	"frps-status-app.local/status/src/config"
	"frps-status-app.local/status/src/frps"
	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/server"
	"frps-status-app.local/status/src/store"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		fatal(err)
	}
	if err := logger.Init(cfg.LogDir); err != nil {
		fatal(err)
	}
	defer logger.Close()

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		fatal(err)
	}
	st := store.New(db)
	if err := st.InitDB(); err != nil {
		fatal(err)
	}
	appcfg := appsettings.New(st)
	if err := appsettings.LoadAppSettings(appcfg); err != nil {
		fatal(err)
	}
	if err := st.SeedUser(cfg.StatusUser, cfg.StatusPassword); err != nil {
		fatal(err)
	}
	fc := frps.NewClient(cfg.FRPSHost, cfg.FRPSDashboardPort, cfg.FRPSDashboardUser, cfg.FRPSDashboardPass, &http.Client{Timeout: 8 * time.Second})
	app := server.New(cfg, st, appcfg, fc)
	app.InitWarnings()
	if err := app.Refresh(context.Background()); err != nil {
		logger.Warn("初始刷新失败: %v", err)
	}
	go app.PollLoop()
	logger.Info("FRPS 状态监控已启动，监听地址: %s", cfg.Listen)
	logger.Info("登录账户: %s  密码: %s", cfg.StatusUser, cfg.StatusPassword)
	if err := http.ListenAndServe(cfg.Listen, app.Routes()); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	logger.Error("%v", err)
	os.Exit(1)
}
