package server

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"frps-status-app.local/status/src/config"
	"frps-status-app.local/status/src/frps"
	"frps-status-app.local/status/src/mail"
	"frps-status-app.local/status/src/model"
	"frps-status-app.local/status/src/store"
)

type App struct {
	cfg    config.Config
	store  *store.Store
	frps   *frps.Client
	mu     sync.RWMutex
	latest model.Snapshot
}

func New(cfg config.Config, st *store.Store, fc *frps.Client) *App {
	return &App{cfg: cfg, store: st, frps: fc}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", a.withAuth(a.handleStatus))
	mux.HandleFunc("/api/daily", a.withAuth(a.handleDaily))
	mux.HandleFunc("/api/daily/export", a.withAuth(a.handleExportCSV))
	mux.HandleFunc("/api/settings", a.withAuth(a.handleSettings))
	mux.HandleFunc("/api/settings/test-email", a.withAuth(a.handleTestEmail))
	mux.HandleFunc("/api/db/vacuum", a.withAuth(a.handleVacuum))
	mux.HandleFunc("/api/db/purge", a.withAuth(a.handlePurge))
	mux.HandleFunc("/", a.withAuth(a.serveIndex))
	return mux
}

func (a *App) PollLoop() {
	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := a.Refresh(context.Background()); err != nil {
			log.Printf("refresh failed: %v", err)
		}
	}
}

func (a *App) Refresh(ctx context.Context) error {
	proxies, err := a.frps.FetchProxies(ctx)
	if err == nil {
		if err := a.store.RecordTraffic(proxies); err != nil {
			log.Printf("record traffic failed: %v", err)
		}
	} else {
		log.Printf("fetch frps proxies failed: %v", err)
	}
	month := time.Now().Format("2006-01")
	for i := range proxies {
		in, out, _ := a.store.MonthTotalForProxy(proxies[i].Name, proxies[i].Type, month)
		proxies[i].MonthIn = in
		proxies[i].MonthOut = out
	}
	settings, _ := a.store.PublicSettings()
	monthIn, monthOut, _ := a.store.MonthTotals(month)
	_ = a.checkAlerts(settings, month, monthIn, monthOut)
	s := model.Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		FRPS: map[string]any{
			"host":           a.cfg.FRPSHost,
			"bind_port":      a.cfg.FRPSBindPort,
			"dashboard_port": a.cfg.FRPSDashboardPort,
			"bind":           frps.CheckTCP(a.cfg.FRPSHost, a.cfg.FRPSBindPort),
			"dashboard":      frps.CheckTCP(a.cfg.FRPSHost, a.cfg.FRPSDashboardPort),
		},
		Certificates: frps.Certificates(a.cfg.CertDir, a.cfg.Domains),
		Proxies:      proxies,
		MonthTotals:  map[string]uint64{"in": monthIn, "out": monthOut},
		Settings:     settings,
	}
	a.mu.Lock()
	a.latest = s
	a.mu.Unlock()
	return err
}

func (a *App) checkAlerts(settings model.PublicSettings, month string, monthIn, monthOut uint64) error {
	if !settings.SMTPEnabled {
		return nil
	}
	if settings.AlertInGB > 0 && monthIn >= mail.GBToBytes(settings.AlertInGB) {
		_ = a.sendAlertOnce(month, "in", monthIn, settings.AlertInGB, settings)
	}
	if settings.AlertOutGB > 0 && monthOut >= mail.GBToBytes(settings.AlertOutGB) {
		_ = a.sendAlertOnce(month, "out", monthOut, settings.AlertOutGB, settings)
	}
	return nil
}

func (a *App) sendAlertOnce(month, direction string, current uint64, thresholdGB float64, settings model.PublicSettings) error {
	if a.store.AlertSent(month, direction) {
		return nil
	}
	password := a.store.Setting("smtp_password")
	if password == "" || settings.SMTPHost == "" || settings.SMTPFrom == "" || settings.SMTPTo == "" {
		return nil
	}
	subject := fmt.Sprintf("FRPS %s traffic alert %s", strings.ToUpper(direction), month)
	body := fmt.Sprintf("FRPS monthly %s traffic is %s, threshold is %.2f GB.", direction, mail.HumanBytes(current), thresholdGB)
	if err := mail.Send(settings, password, subject, body); err != nil {
		return err
	}
	return a.store.MarkAlertSent(month, direction)
}

func (a *App) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.authorized(r) {
			next(w, r)
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="FRPS Status"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (a *App) authorized(r *http.Request) bool {
	if a.cfg.StatusUser == "" && a.cfg.StatusPassword == "" {
		return true
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Basic ") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
	if err != nil {
		return false
	}
	expected := []byte(a.cfg.StatusUser + ":" + a.cfg.StatusPassword)
	if len(raw) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(raw, expected) == 1
}

func (a *App) serveIndex(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	path = filepath.Clean(path)
	if strings.HasPrefix(path, "..") {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join("/app/web", path)
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.ServeFile(w, r, "/app/web/index.html")
			return
		}
		http.Error(w, err.Error(), 500)
		return
	}
	http.ServeFile(w, r, full)
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = a.Refresh(r.Context())
	a.mu.RLock()
	s := a.latest
	a.mu.RUnlock()
	writeJSON(w, s)
}

func (a *App) handleDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := a.store.DailyTraffic()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, _ := a.store.PublicSettings()
		writeJSON(w, settings)
	case http.MethodPost:
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		allowed := map[string]bool{"alert_in_gb": true, "alert_out_gb": true, "smtp_host": true, "smtp_port": true, "smtp_user": true, "smtp_password": true, "smtp_from": true, "smtp_to": true, "smtp_enabled": true}
		for key, value := range in {
			if !allowed[key] {
				continue
			}
			if key == "smtp_password" && fmt.Sprint(value) == "" {
				continue
			}
			if err := a.store.SaveSetting(key, fmt.Sprint(value)); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
		}
		settings, _ := a.store.PublicSettings()
		writeJSON(w, settings)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings, _ := a.store.PublicSettings()
	if !settings.SMTPEnabled {
		writeJSON(w, map[string]any{"ok": false, "error": "SMTP 未启用"})
		return
	}
	password := a.store.Setting("smtp_password")
	if err := mail.Send(settings, password, "FRPS Status - 测试邮件", "这是一封来自 FRPS Status 的测试邮件，SMTP 配置正常。"); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleVacuum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.store.Vacuum(); err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Days < 1 {
		body.Days = 30
	}
	deleted, err := a.store.Purge(body.Days)
	if err != nil {
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "deleted": deleted})
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.store.DailyTraffic()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="frps-traffic.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"day", "proxy_name", "proxy_type", "in_bytes", "out_bytes"})
	for _, row := range rows {
		_ = cw.Write([]string{
			fmt.Sprint(row["day"]),
			fmt.Sprint(row["name"]),
			fmt.Sprint(row["type"]),
			strconv.FormatInt(int64(row["in"].(int64)), 10),
			strconv.FormatInt(int64(row["out"].(int64)), 10),
		})
	}
	cw.Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}
