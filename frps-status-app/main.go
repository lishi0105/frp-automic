package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"frps-status-app/src/config"
	"frps-status-app/src/frps"
	"frps-status-app/src/server"
	"frps-status-app/src/store"
)

func main() {
	cfg := config.Load()
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		log.Fatal(err)
	}
	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	st := store.New(db)
	if err := st.InitDB(); err != nil {
		log.Fatal(err)
	}
	fc := frps.NewClient(cfg.FRPSHost, cfg.FRPSDashboardPort, cfg.FRPSDashboardUser, cfg.FRPSDashboardPass, &http.Client{Timeout: 8 * time.Second})
	app := server.New(cfg, st, fc)
	if err := app.Refresh(context.Background()); err != nil {
		log.Printf("initial refresh failed: %v", err)
	}
	go app.PollLoop()
	log.Printf("frps status listening on %s", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, app.Routes()))
}
