package server

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
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
	secret []byte
	mu     sync.RWMutex
	latest model.Snapshot
}

func New(cfg config.Config, st *store.Store, fc *frps.Client) *App {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		secret = []byte(cfg.StatusUser + ":" + cfg.StatusPassword + ":" + cfg.Listen)
	}
	return &App{cfg: cfg, store: st, frps: fc, secret: secret}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", a.handleLogin)
	mux.HandleFunc("/api/logout", a.handleLogout)
	mux.HandleFunc("/api/session", a.handleSession)
	mux.HandleFunc("/api/status", a.withAuth(a.handleStatus))
	mux.HandleFunc("/api/daily", a.withAuth(a.handleDaily))
	mux.HandleFunc("/api/daily/export", a.withAuth(a.handleExportCSV))
	mux.HandleFunc("/api/settings", a.withAuth(a.handleSettings))
	mux.HandleFunc("/api/settings/test-email", a.withAuth(a.handleTestEmail))
	mux.HandleFunc("/api/db/vacuum", a.withAuth(a.handleVacuum))
	mux.HandleFunc("/api/db/purge", a.withAuth(a.handlePurge))
	mux.HandleFunc("/", a.serveIndex)
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
	proxies, fetchErr := a.frps.FetchProxies(ctx)
	if fetchErr == nil {
		if err := a.store.RecordTraffic(proxies); err != nil {
			log.Printf("record traffic failed: %v", err)
		}
	} else {
		log.Printf("fetch frps proxies failed: %v", fetchErr)
	}
	month := time.Now().Format("2006-01")
	for i := range proxies {
		in, out, _ := a.store.MonthTotalForProxy(proxies[i].Name, proxies[i].Type, month)
		proxies[i].MonthIn = in
		proxies[i].MonthOut = out
	}
	settings, _ := a.store.PublicSettings()
	monthIn, monthOut, _ := a.store.MonthTotals(month)
	certs := frps.Certificates(a.cfg.CertDir, a.cfg.Domains)
	_ = a.checkAlerts(settings, month, monthIn, monthOut)
	if fetchErr == nil {
		a.checkProxyAlerts(settings, proxies)
	}
	a.checkCertAlerts(settings, certs)
	s := model.Snapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		FRPS: map[string]any{
			"host":           a.cfg.FRPSHost,
			"bind_port":      a.cfg.FRPSBindPort,
			"dashboard_port": a.cfg.FRPSDashboardPort,
			"bind":           frps.CheckTCP(a.cfg.FRPSHost, a.cfg.FRPSBindPort),
			"dashboard":      frps.CheckTCP(a.cfg.FRPSHost, a.cfg.FRPSDashboardPort),
		},
		Certificates: certs,
		Proxies:      proxies,
		MonthTotals:  map[string]uint64{"in": monthIn, "out": monthOut},
		Settings:     settings,
	}
	a.mu.Lock()
	a.latest = s
	a.mu.Unlock()
	return fetchErr
}

func (a *App) checkAlerts(settings model.PublicSettings, month string, monthIn, monthOut uint64) error {
	if !settings.SMTPEnabled {
		return nil
	}
	total := monthIn + monthOut
	if settings.AlertInGB > 0 && monthIn >= mail.GBToBytes(settings.AlertInGB) {
		_ = a.sendAlertOnce(month, "in", monthIn, settings.AlertInGB, settings)
	}
	if settings.AlertOutGB > 0 && monthOut >= mail.GBToBytes(settings.AlertOutGB) {
		_ = a.sendAlertOnce(month, "out", monthOut, settings.AlertOutGB, settings)
	}
	if settings.AlertTotalGB > 0 && total >= mail.GBToBytes(settings.AlertTotalGB) {
		_ = a.sendAlertOnce(month, "total", total, settings.AlertTotalGB, settings)
	}
	return nil
}

func (a *App) sendAlertOnce(month, direction string, current uint64, thresholdGB float64, settings model.PublicSettings) error {
	if a.store.AlertSent(month, direction) {
		return nil
	}
	authCode := a.store.Setting("smtp_auth_code")
	if authCode == "" || settings.SMTPHost == "" || settings.SMTPFrom == "" || settings.SMTPTo == "" {
		return nil
	}
	subject := fmt.Sprintf("FRPS %s traffic alert %s", strings.ToUpper(direction), month)
	body := fmt.Sprintf("FRPS monthly %s traffic is %s, threshold is %.2f GB.", direction, mail.HumanBytes(current), thresholdGB)
	if err := mail.Send(settings, authCode, subject, body); err != nil {
		return err
	}
	return a.store.MarkAlertSent(month, direction)
}

func (a *App) checkProxyAlerts(settings model.PublicSettings, proxies []model.ProxyTraffic) {
	if !settings.SMTPEnabled {
		return
	}
	authCode := a.store.Setting("smtp_auth_code")
	if authCode == "" || settings.SMTPHost == "" || settings.SMTPFrom == "" || settings.SMTPTo == "" {
		return
	}
	for _, p := range proxies {
		key := "proxy_offline:" + p.Type + ":" + p.Name
		if !p.Online {
			if !a.store.EventAlertSent(key) {
				subject := fmt.Sprintf("FRPS 代理离线告警 - %s", p.Name)
				body := fmt.Sprintf("代理 %s（类型：%s）已离线，请检查客户端连接状态。", p.Name, p.Type)
				if err := mail.Send(settings, authCode, subject, body); err != nil {
					log.Printf("send proxy offline alert failed: %v", err)
					continue
				}
				_ = a.store.SetEventAlert(key)
			}
		} else {
			_ = a.store.ClearEventAlert(key)
		}
	}
}

func (a *App) checkCertAlerts(settings model.PublicSettings, certs []model.CertStatus) {
	if !settings.SMTPEnabled {
		return
	}
	authCode := a.store.Setting("smtp_auth_code")
	if authCode == "" || settings.SMTPHost == "" || settings.SMTPFrom == "" || settings.SMTPTo == "" {
		return
	}
	for _, c := range certs {
		key := "cert_expiry:" + c.Domain
		if c.Present && c.DaysLeft != nil && !c.OK {
			if !a.store.EventAlertSent(key) {
				subject := fmt.Sprintf("FRPS SSL证书即将到期 - %s", c.Domain)
				body := fmt.Sprintf("域名 %s 的 SSL 证书将在 %d 天后到期（到期时间：%s），请及时续期。", c.Domain, *c.DaysLeft, c.ExpiresAt)
				if err := mail.Send(settings, authCode, subject, body); err != nil {
					log.Printf("send cert expiry alert failed: %v", err)
					continue
				}
				_ = a.store.SetEventAlert(key)
			}
		} else if c.OK {
			_ = a.store.ClearEventAlert(key)
		}
	}
}

func (a *App) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.authorized(r) {
			next(w, r)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (a *App) authorized(r *http.Request) bool {
	if a.cfg.StatusUser == "" && a.cfg.StatusPassword == "" {
		return true
	}
	if a.validSession(r) {
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

func (a *App) checkCredentials(user, password string) bool {
	expected := []byte(a.cfg.StatusUser + ":" + a.cfg.StatusPassword)
	actual := []byte(user + ":" + password)
	if len(actual) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func (a *App) validSession(r *http.Request) bool {
	cookie, err := r.Cookie("frps_status_session")
	if err != nil || cookie.Value == "" {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return false
	}
	expiry, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	msg := parts[0] + "." + parts[1]
	expected := a.signSession(msg)
	return hmac.Equal([]byte(parts[2]), []byte(expected))
}

func (a *App) newSessionCookie() *http.Cookie {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		nonce = []byte(strconv.FormatInt(time.Now().UnixNano(), 10))
	}
	expiry := time.Now().Add(12 * time.Hour).Unix()
	msg := fmt.Sprintf("%d.%s", expiry, hex.EncodeToString(nonce))
	return &http.Cookie{
		Name:     "frps_status_session",
		Value:    msg + "." + a.signSession(msg),
		Path:     "/",
		MaxAge:   12 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (a *App) signSession(msg string) string {
	mac := hmac.New(sha256.New, a.secret)
	_, _ = mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.cfg.StatusUser == "" && a.cfg.StatusPassword == "" {
		http.SetCookie(w, a.newSessionCookie())
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.checkCredentials(in.Username, in.Password) {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "用户名或密码不正确"})
		return
	}
	http.SetCookie(w, a.newSessionCookie())
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "frps_status_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, map[string]any{"authenticated": a.authorized(r)})
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
		allowed := map[string]bool{"alert_in_gb": true, "alert_out_gb": true, "alert_total_gb": true, "smtp_host": true, "smtp_port": true, "smtp_user": true, "smtp_auth_code": true, "smtp_from": true, "smtp_to": true, "smtp_enabled": true}
		for key, value := range in {
			if !allowed[key] {
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
	authCode := a.store.Setting("smtp_auth_code")
	if err := mail.Send(settings, authCode, "FRPS Status - 测试邮件", "这是一封来自 FRPS Status 的测试邮件，SMTP 配置正常。"); err != nil {
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

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
