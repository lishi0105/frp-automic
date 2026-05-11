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
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"frps-status-app.local/status/src/alerting"
	"frps-status-app.local/status/src/appsettings"
	"frps-status-app.local/status/src/config"
	"frps-status-app.local/status/src/frps"
	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/mail"
	"frps-status-app.local/status/src/model"
	"frps-status-app.local/status/src/monitor"
	"frps-status-app.local/status/src/store"
)

type App struct {
	cfg              config.Config
	store            *store.Store
	appcfg           *appsettings.Manager
	frps             *frps.Client
	alerts           *alerting.Manager
	secret           []byte
	mu               sync.RWMutex
	latest           model.Snapshot
	lastAutoPurgeDay string
	// storageOpsMu 串行「自动历史清理 + 磁盘空间检测/应急」与「手动存储清理」，避免 Purge/VACUUM/删日志并发冲突。
	storageOpsMu sync.Mutex
}

func New(cfg config.Config, st *store.Store, appcfg *appsettings.Manager, fc *frps.Client) *App {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		secret = []byte(cfg.StatusUser + ":" + cfg.StatusPassword + ":" + cfg.Listen)
	}
	return &App{cfg: cfg, store: st, appcfg: appcfg, frps: fc, alerts: alerting.New(st), secret: secret}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", a.handleLogin)
	mux.HandleFunc("/api/logout", a.handleLogout)
	mux.HandleFunc("/api/session", a.handleSession)
	mux.HandleFunc("/api/user/forgot-password", a.handleForgotPassword)
	mux.HandleFunc("/api/user", a.withAuth(a.handleUser))
	mux.HandleFunc("/api/user/credentials", a.withAuth(a.handleChangeCredentials))
	mux.HandleFunc("/api/user/recovery-email", a.withAuth(a.handleChangeRecoveryEmail))
	mux.HandleFunc("/api/warnings", a.withAuth(a.handleWarnings))
	mux.HandleFunc("/api/status", a.withAuth(a.handleStatus))
	mux.HandleFunc("/api/host-network", a.withAuth(a.handleHostNetwork))
	mux.HandleFunc("/api/storage", a.withAuth(a.handleStorage))
	mux.HandleFunc("/api/storage/app-usage", a.withAuth(a.handleStorageAppUsage))
	mux.HandleFunc("/api/storage/cleanup", a.withAuth(a.handleStorageCleanup))
	mux.HandleFunc("/api/proxies", a.withAuth(a.handleProxies))
	mux.HandleFunc("/api/certificates", a.withAuth(a.handleCertificates))
	mux.HandleFunc("/api/daily", a.withAuth(a.handleDaily))
	mux.HandleFunc("/api/daily/interface", a.withAuth(a.handleDailyInterface))
	mux.HandleFunc("/api/daily/export", a.withAuth(a.handleExportCSV))
	mux.HandleFunc("/api/settings", a.withAuth(a.handleSettings))
	mux.HandleFunc("/api/settings/test-email", a.withAuth(a.handleTestEmail))
	mux.HandleFunc("/api/db/vacuum", a.withAuth(a.handleVacuum))
	mux.HandleFunc("/api/db/purge", a.withAuth(a.handlePurge))
	mux.HandleFunc("/api/logs/current", a.withAuth(a.handleCurrentLog))
	mux.HandleFunc("/api/logs/clear", a.withAuth(a.handleClearLog))
	mux.HandleFunc("/", a.serveIndex)
	return mux
}

func (a *App) PollLoop() {
	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := a.Refresh(context.Background()); err != nil {
			logger.Warn("定时刷新失败: %v", err)
		}
		func() {
			a.storageOpsMu.Lock()
			defer a.storageOpsMu.Unlock()
			a.autoPurgeHistoryIfNeeded()
			a.checkDiskSpaceRoutine()
		}()
	}
}

func (a *App) autoPurgeHistoryIfNeeded() {
	now := time.Now()
	hour := now.Hour()
	if hour < 2 || hour >= 4 {
		return
	}
	day := now.Format("2006-01-02")
	a.mu.RLock()
	alreadyDone := a.lastAutoPurgeDay == day
	a.mu.RUnlock()
	if alreadyDone {
		return
	}
	settings := a.appcfg.PublicSettings()
	days := settings.HistoryRetentionDays
	if days < 1 {
		days = 60
	}
	deleted, err := a.store.Purge(days)
	if err != nil {
		logger.Error("自动清理历史数据失败 保留天数=%d 错误=%v", days, err)
		return
	}
	a.mu.Lock()
	a.lastAutoPurgeDay = day
	a.mu.Unlock()
	logger.Info("自动清理历史数据完成 日期=%s 保留天数=%d 已删除=%d", day, days, deleted)
}

func (a *App) Refresh(ctx context.Context) error {
	proxies, fetchErr := a.frps.FetchProxies(ctx)
	if fetchErr == nil {
		if err := a.store.RecordTraffic(proxies); err != nil {
			logger.Error("记录流量数据失败: %v", err)
		}
	} else {
		logger.Warn("获取 FRPS 代理列表失败: %v", fetchErr)
	}
	month := time.Now().Format("2006-01")
	for i := range proxies {
		in, out, _ := a.store.MonthTotalForProxy(proxies[i].Name, proxies[i].Type, month)
		proxies[i].MonthIn = in
		proxies[i].MonthOut = out
	}
	settings := a.appcfg.PublicSettings()
	a.collectInterfaceTraffic()
	monthIn, monthOut, _ := a.store.MonthInterfaceTotals(month)
	monthIn, monthOut = applyInitialTrafficToMonth(settings, month, monthIn, monthOut)
	certs := frps.Certificates(a.cfg.CertDir, a.cfg.Domains)
	inferProxyCertificateDomains(proxies, certs)
	if fetchErr == nil {
		a.enrichProxyHealth(proxies)
	}
	certThreshold := settings.AlertCertDays
	if certThreshold <= 0 {
		certThreshold = 15
	}
	proxyFetchError := ""
	if fetchErr != nil {
		proxyFetchError = fetchErr.Error()
	}
	linkCtx, linkCancel := context.WithTimeout(ctx, 15*time.Second)
	linkHealth := monitor.ProbeLinkHealth(linkCtx)
	linkCancel()
	hostPressure := monitor.ProbeHostPressure(a.cfg.ProcRoot, a.cfg.DBPath)
	a.alerts.Process(settings, monitor.Result{
		LinkProbed:      true,
		LinkHealth:      linkHealth,
		HostPressure:    hostPressure,
		ProxyFetchError: proxyFetchError,
		Proxies:         proxies,
		Certificates:    certs,
		Traffic:         monitor.BuildTrafficResult(settings, month, monthIn, monthOut),
		CertThreshold:   certThreshold,
	})
	a.syncTrafficWarnings(settings, monthIn, monthOut)
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
		Dashboard: model.DashboardSummary{
			TopProxies:  buildTopProxies(proxies, 5),
			Certificate: summarizeCertificates(certs),
		},
		Settings: settings,
	}
	a.mu.Lock()
	a.latest = s
	a.mu.Unlock()
	return fetchErr
}

func (a *App) collectInterfaceTraffic() {
	publicIP := strings.TrimSpace(a.cfg.HostPublicIP)
	iface := strings.TrimSpace(a.cfg.HostIface)

	if publicIP == "" {
		detectedIP, err := detectOutboundPublicIP()
		if err != nil || detectedIP == "" {
			if err != nil {
				logger.Warn("识别公网出口IP失败: %v", err)
			}
			return
		}
		publicIP = detectedIP
	}

	if iface == "" {
		detectedIface, err := interfaceByIP(publicIP)
		if err != nil || detectedIface == "" {
			if err != nil {
				logger.Warn("根据公网IP匹配网卡失败 IP=%s 错误=%v", publicIP, err)
			}
			return
		}
		iface = detectedIface
	}

	rxBytes, txBytes, err := readInterfaceCounters(a.cfg.HostNetStatsDir, iface)
	if err != nil {
		logger.Warn("读取网卡计数失败 网卡=%s 错误=%v", iface, err)
		return
	}
	if err := a.store.RecordInterfaceTraffic(iface, publicIP, rxBytes, txBytes, time.Now()); err != nil {
		logger.Error("记录网卡日流量失败 网卡=%s IP=%s 错误=%v", iface, publicIP, err)
	}
}

func inferProxyCertificateDomains(proxies []model.ProxyTraffic, certs []model.CertStatus) {
	certDomainsByAlias := make(map[string][]string, len(certs))
	for _, cert := range certs {
		domain := strings.TrimSpace(strings.ToLower(cert.Domain))
		if domain == "" {
			continue
		}
		alias := domain
		if dot := strings.IndexByte(domain, '.'); dot >= 0 {
			alias = domain[:dot]
		}
		for _, key := range aliasMatchKeys(alias) {
			certDomainsByAlias[key] = append(certDomainsByAlias[key], domain)
		}
	}

	for i := range proxies {
		seen := make(map[string]struct{}, len(proxies[i].Domains))
		for _, domain := range proxies[i].Domains {
			domain = strings.TrimSpace(strings.ToLower(domain))
			if domain == "" {
				continue
			}
			seen[domain] = struct{}{}
		}
		for _, key := range aliasMatchKeys(proxies[i].Name) {
			for _, domain := range certDomainsByAlias[key] {
				seen[domain] = struct{}{}
			}
		}
		if len(seen) == 0 {
			continue
		}
		domains := make([]string, 0, len(seen))
		for domain := range seen {
			domains = append(domains, domain)
		}
		sort.Strings(domains)
		proxies[i].Domains = domains
	}
}

func aliasMatchKeys(alias string) []string {
	alias = strings.TrimSpace(strings.ToLower(alias))
	if alias == "" {
		return nil
	}
	keys := []string{alias}
	hyphen := strings.ReplaceAll(alias, "_", "-")
	if hyphen != alias {
		keys = append(keys, hyphen)
	}
	underscore := strings.ReplaceAll(alias, "-", "_")
	if underscore != alias && underscore != hyphen {
		keys = append(keys, underscore)
	}
	return keys
}

func buildTopProxies(proxies []model.ProxyTraffic, limit int) []model.DashboardTopProxy {
	if limit <= 0 {
		limit = 5
	}
	items := make([]model.DashboardTopProxy, 0, len(proxies))
	for _, p := range proxies {
		total := p.MonthIn + p.MonthOut
		items = append(items, model.DashboardTopProxy{
			Name:     p.Name,
			Type:     p.Type,
			MonthIn:  p.MonthIn,
			MonthOut: p.MonthOut,
			Total:    total,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Total == items[j].Total {
			return items[i].Name < items[j].Name
		}
		return items[i].Total > items[j].Total
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func summarizeCertificates(certs []model.CertStatus) model.DashboardCertSummary {
	out := model.DashboardCertSummary{Total: len(certs)}
	for _, c := range certs {
		if !c.Present || !c.OK {
			out.Fail++
			continue
		}
		if c.DaysLeft != nil && *c.DaysLeft < 15 {
			out.Warn++
		} else {
			out.OK++
		}
		if c.DaysLeft != nil {
			if out.MinDaysLeft == nil || *c.DaysLeft < *out.MinDaysLeft {
				v := *c.DaysLeft
				out.MinDaysLeft = &v
				out.MinDomain = c.Domain
			}
		}
	}
	return out
}

func (a *App) enrichProxyHealth(proxies []model.ProxyTraffic) {
	now := time.Now().UTC()
	since := now.Add(-24 * time.Hour)
	for i := range proxies {
		key := "proxy_offline:" + proxies[i].Type + ":" + proxies[i].Name
		st, err := a.store.GetEventState(key)
		if err != nil {
			continue
		}
		prevOffline := st.FailStreak > 0
		st.Key = key
		st.TotalChecks++
		if proxies[i].Online {
			st.OnlineChecks++
			st.FailStreak = 0
			st.FirstFailAt = ""
			if prevOffline {
				st.RecoverStreak = 1
				st.RecoverSince = now.Format(time.RFC3339)
			} else {
				st.RecoverStreak++
				if st.RecoverSince == "" {
					st.RecoverSince = now.Format(time.RFC3339)
				}
			}
		} else {
			st.FailStreak++
			st.RecoverStreak = 0
			st.RecoverSince = ""
			if st.FirstFailAt == "" {
				st.FirstFailAt = now.Format(time.RFC3339)
			}
			st.LastOfflineAt = now.Format(time.RFC3339)
		}
		if prevOffline != !proxies[i].Online {
			st.LastChangeAt = now.Format(time.RFC3339)
			if proxies[i].Online {
				st.LastRecoveryAt = now.Format(time.RFC3339)
			}
			_ = a.store.AddProxyStatusEvent(key, proxies[i].Online, now)
		}
		st.LastSeenAt = now.Format(time.RFC3339)
		if err := a.store.SaveEventState(st); err != nil {
			continue
		}
		rate := 0
		if st.TotalChecks > 0 {
			rate = int((st.OnlineChecks * 100) / st.TotalChecks)
		}
		flaps, _ := a.store.ProxyFlapCountSince(key, since)
		health := model.ProxyHealth{
			ConsecutiveOffline: st.FailStreak,
			OnlineChecks:       st.OnlineChecks,
			TotalChecks:        st.TotalChecks,
			OnlineRate:         rate,
			FlapCount24h:       flaps,
			LastChangeAt:       st.LastChangeAt,
			LastOfflineAt:      st.LastOfflineAt,
			LastRecoveryAt:     st.LastRecoveryAt,
			RecoveryConfirmed:  st.RecoveryConfirmed(now),
		}
		if !proxies[i].Online && st.FirstFailAt != "" {
			if t, err := time.Parse(time.RFC3339, st.FirstFailAt); err == nil {
				health.OfflineSeconds = int64(now.Sub(t).Seconds())
			}
		}
		proxies[i].Health = health
	}
}

func (a *App) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.authorized(r) {
			next(w, r)
			return
		}
		logger.Warn("未授权请求 方法=%s 路径=%s 来源=%s", r.Method, r.URL.Path, r.RemoteAddr)
		w.WriteHeader(http.StatusUnauthorized)
	}
}

func (a *App) authorized(r *http.Request) bool {
	if a.validSession(r) {
		return true
	}
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Basic ") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header, "Basic "))
	if err != nil {
		logger.Warn("无效的 Basic Auth 请求头 来源=%s 错误=%v", r.RemoteAddr, err)
		return false
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 {
		logger.Warn("Basic Auth 凭据格式错误 来源=%s", r.RemoteAddr)
		return false
	}
	return a.checkCredentials(parts[0], parts[1])
}

func (a *App) checkCredentials(username, password string) bool {
	u, err := a.store.GetUser()
	if err != nil {
		logger.Error("凭据验证时获取用户失败: %v", err)
		return false
	}
	if subtle.ConstantTimeCompare([]byte(u.Username), []byte(username)) == 0 {
		logger.Warn("凭据验证失败：用户名不匹配 用户名=%s", username)
		return false
	}
	expected := store.HashPassword(u.PasswordSalt, password)
	ok := subtle.ConstantTimeCompare([]byte(expected), []byte(u.PasswordHash)) == 1
	if !ok {
		logger.Warn("凭据验证失败：密码不匹配 用户名=%s", username)
	}
	return ok
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
		logger.Warn("Session Cookie 无效或已过期 来源=%s", r.RemoteAddr)
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
		logger.Warn("登录请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		logger.Warn("登录请求体解析失败 来源=%s 错误=%v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !a.checkCredentials(in.Username, in.Password) {
		logger.Warn("登录失败 用户名=%s 来源=%s", in.Username, r.RemoteAddr)
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "用户名或密码不正确"})
		return
	}
	http.SetCookie(w, a.newSessionCookie())
	u, _ := a.store.GetUser()
	logger.Info("登录成功 用户名=%s 来源=%s", in.Username, r.RemoteAddr)
	writeJSON(w, map[string]any{"ok": true, "force_change": u.IsInitialPassword})
}

func (a *App) handleUser(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("获取用户请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	u, err := a.store.GetUser()
	if err != nil {
		logger.Error("获取用户信息失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"id":                  u.ID,
		"username":            u.Username,
		"recovery_email":      u.RecoveryEmail,
		"is_initial_password": u.IsInitialPassword,
	})
}

func (a *App) handleChangeCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("修改凭据请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		logger.Warn("修改凭据请求体解析失败 来源=%s 错误=%v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := a.store.GetUser()
	if err != nil {
		logger.Error("修改凭据时获取用户失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if subtle.ConstantTimeCompare([]byte(store.HashPassword(u.PasswordSalt, in.CurrentPassword)), []byte(u.PasswordHash)) == 0 {
		logger.Warn("修改凭据被拒绝：当前密码不匹配 用户名=%s 来源=%s", u.Username, r.RemoteAddr)
		writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "当前密码不正确"})
		return
	}
	targetUsername := strings.TrimSpace(in.Username)
	if targetUsername == "" {
		targetUsername = u.Username
	}
	if err := validateUsername(targetUsername); err != nil {
		logger.Warn("修改凭据时用户名验证失败 用户名=%s 错误=%v", targetUsername, err)
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	salt := u.PasswordSalt
	hash := u.PasswordHash
	if in.NewPassword != "" {
		if err := validatePassword(in.NewPassword); err != nil {
			logger.Warn("修改凭据时密码验证失败 用户名=%s 错误=%v", targetUsername, err)
			writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		newSalt, err := store.GenerateSalt()
		if err != nil {
			logger.Error("修改凭据时生成密码盐失败 用户名=%s 错误=%v", targetUsername, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		salt = newSalt
		hash = store.HashPassword(newSalt, in.NewPassword)
	}
	if err := a.store.UpdateUserCredentials(u.ID, targetUsername, hash, salt); err != nil {
		logger.Error("更新用户凭据失败 用户名=%s 错误=%v", targetUsername, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Info("用户凭据已更新 旧用户名=%s 新用户名=%s 密码已修改=%t", u.Username, targetUsername, in.NewPassword != "")
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleChangeRecoveryEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("修改找回邮箱请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		RecoveryEmail string `json:"recovery_email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		logger.Warn("修改找回邮箱请求体解析失败 来源=%s 错误=%v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	u, err := a.store.GetUser()
	if err != nil {
		logger.Error("修改找回邮箱时获取用户失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.UpdateUserRecoveryEmail(u.ID, strings.TrimSpace(in.RecoveryEmail)); err != nil {
		logger.Error("更新找回邮箱失败 用户名=%s 错误=%v", u.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	a.syncRecoveryEmailWarning()
	logger.Info("找回邮箱已更新 用户名=%s 是否有邮箱=%t", u.Username, strings.TrimSpace(in.RecoveryEmail) != "")
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleForgotPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("忘记密码请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		logger.Warn("忘记密码请求体解析失败 来源=%s 错误=%v", r.RemoteAddr, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(in.Email)
	if email == "" {
		logger.Warn("忘记密码被拒绝：邮箱为空 来源=%s", r.RemoteAddr)
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "请输入找回邮箱"})
		return
	}
	u, err := a.store.GetUser()
	if err != nil || strings.EqualFold(u.RecoveryEmail, "") || !strings.EqualFold(u.RecoveryEmail, email) {
		if err != nil {
			logger.Error("忘记密码时获取用户失败: %v", err)
		} else {
			logger.Warn("忘记密码被拒绝：邮箱不匹配 来源=%s", r.RemoteAddr)
		}
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "邮箱不匹配，请联系管理员"})
		return
	}
	settings := a.appcfg.PublicSettings()
	if !settings.SMTPEnabled || settings.SMTPHost == "" || settings.SMTPAuthCode == "" {
		logger.Warn("忘记密码被拒绝：SMTP 配置不完整 用户名=%s", u.Username)
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "SMTP 未配置，无法发送重置邮件"})
		return
	}
	newPass, err := generatePassword(16)
	if err != nil {
		logger.Error("忘记密码时生成新密码失败 用户名=%s 错误=%v", u.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newSalt, err := store.GenerateSalt()
	if err != nil {
		logger.Error("忘记密码时生成密码盐失败 用户名=%s 错误=%v", u.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := a.store.UpdateUserCredentials(u.ID, u.Username, store.HashPassword(newSalt, newPass), newSalt); err != nil {
		logger.Error("忘记密码时更新凭据失败 用户名=%s 错误=%v", u.Username, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	subject := "FRPS状态监控 - 密码重置通知"
	body := fmt.Sprintf("您的账户凭据已重置：\n\n用户名：%s\n密　码：%s\n\n请登录后及时修改密码。", u.Username, newPass)
	if err := mail.SendTo(settings, settings.SMTPAuthCode, email, subject, body); err != nil {
		logger.Error("忘记密码时发送重置邮件失败 用户名=%s 错误=%v", u.Username, err)
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "邮件发送失败：" + err.Error()})
		return
	}
	logger.Info("忘记密码重置邮件已发送 用户名=%s", u.Username)
	writeJSON(w, map[string]any{"ok": true})
}

func generatePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789*#!()"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		logger.Error("生成随机密码时读取随机数失败: %v", err)
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("退出登录请求被拒绝：请求方法不允许 方法=%s", r.Method)
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
	logger.Info("退出登录成功 来源=%s", r.RemoteAddr)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("Session 检查请求被拒绝：请求方法不允许 方法=%s", r.Method)
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
		logger.Warn("拦截可疑静态文件路径 路径=%s 来源=%s", r.URL.Path, r.RemoteAddr)
		http.NotFound(w, r)
		return
	}
	full := filepath.Join("/app/web", path)
	if _, err := os.Stat(full); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.ServeFile(w, r, "/app/web/index.html")
			return
		}
		logger.Error("检查静态文件状态失败 路径=%s 错误=%v", full, err)
		http.Error(w, err.Error(), 500)
		return
	}
	http.ServeFile(w, r, full)
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("状态查询请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.Refresh(r.Context()); err != nil {
		logger.Warn("状态刷新失败: %v", err)
	}
	a.mu.RLock()
	s := a.latest
	a.mu.RUnlock()
	writeJSON(w, s)
}

func (a *App) handleHostNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	publicIP := strings.TrimSpace(a.cfg.HostPublicIP)
	iface := strings.TrimSpace(a.cfg.HostIface)

	if publicIP == "" {
		ip, err := detectOutboundPublicIP()
		if err == nil {
			publicIP = ip
		}
	}
	if iface == "" && publicIP != "" {
		name, err := interfaceByIP(publicIP)
		if err == nil {
			iface = name
		}
	}
	writeJSON(w, map[string]any{
		"public_ip": publicIP,
		"iface":     iface,
	})
}

func (a *App) handleProxies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.Refresh(r.Context()); err != nil {
		logger.Warn("代理分页查询刷新失败: %v", err)
	}
	q := r.URL.Query()
	page := clampInt(queryInt(q.Get("page"), 1), 1, 100000)
	pageSize := clampInt(queryInt(q.Get("page_size"), 10), 1, 200)
	sortKey := strings.TrimSpace(strings.ToLower(q.Get("sort")))
	if sortKey == "" {
		sortKey = "total"
	}
	order := strings.TrimSpace(strings.ToLower(q.Get("order")))
	if order != "asc" {
		order = "desc"
	}
	keyword := strings.TrimSpace(strings.ToLower(q.Get("keyword")))
	typeFilter := strings.TrimSpace(strings.ToLower(q.Get("type")))
	onlineRaw := strings.TrimSpace(strings.ToLower(q.Get("online")))

	a.mu.RLock()
	all := append([]model.ProxyTraffic(nil), a.latest.Proxies...)
	a.mu.RUnlock()

	filtered := make([]model.ProxyTraffic, 0, len(all))
	for _, p := range all {
		if typeFilter != "" && strings.ToLower(p.Type) != typeFilter {
			continue
		}
		if onlineRaw == "online" && !p.Online {
			continue
		}
		if onlineRaw == "offline" && p.Online {
			continue
		}
		if keyword != "" {
			hit := strings.Contains(strings.ToLower(p.Name), keyword)
			if !hit {
				for _, d := range p.Domains {
					if strings.Contains(strings.ToLower(d), keyword) {
						hit = true
						break
					}
				}
			}
			if !hit {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	sort.Slice(filtered, func(i, j int) bool {
		pi, pj := filtered[i], filtered[j]
		cmp := 0
		switch sortKey {
		case "name":
			cmp = strings.Compare(pi.Name, pj.Name)
		case "type":
			cmp = strings.Compare(pi.Type, pj.Type)
		case "status":
			si, sj := 0, 0
			if pi.Online {
				si = 1
			}
			if pj.Online {
				sj = 1
			}
			cmp = si - sj
			if cmp == 0 {
				cmp = strings.Compare(pi.Name, pj.Name)
			}
		case "conn":
			switch {
			case pi.CurConns < pj.CurConns:
				cmp = -1
			case pi.CurConns > pj.CurConns:
				cmp = 1
			default:
				cmp = strings.Compare(pi.Name, pj.Name)
			}
		case "in":
			switch {
			case pi.MonthIn < pj.MonthIn:
				cmp = -1
			case pi.MonthIn > pj.MonthIn:
				cmp = 1
			default:
				cmp = strings.Compare(pi.Name, pj.Name)
			}
		case "out":
			switch {
			case pi.MonthOut < pj.MonthOut:
				cmp = -1
			case pi.MonthOut > pj.MonthOut:
				cmp = 1
			default:
				cmp = strings.Compare(pi.Name, pj.Name)
			}
		default:
			ti, tj := pi.MonthIn+pi.MonthOut, pj.MonthIn+pj.MonthOut
			switch {
			case ti < tj:
				cmp = -1
			case ti > tj:
				cmp = 1
			default:
				cmp = strings.Compare(pi.Name, pj.Name)
			}
		}
		if order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	items, meta := paginateProxies(filtered, page, pageSize)
	writeJSON(w, model.ProxyListResponse{Items: items, Meta: meta})
}

func (a *App) handleCertificates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.Refresh(r.Context()); err != nil {
		logger.Warn("证书分页查询刷新失败: %v", err)
	}
	q := r.URL.Query()
	page := clampInt(queryInt(q.Get("page"), 1), 1, 100000)
	pageSize := clampInt(queryInt(q.Get("page_size"), 10), 1, 200)
	sortKey := strings.TrimSpace(strings.ToLower(q.Get("sort")))
	if sortKey == "" {
		sortKey = "days"
	}
	order := strings.TrimSpace(strings.ToLower(q.Get("order")))
	if order != "desc" {
		order = "asc"
	}
	keyword := strings.TrimSpace(strings.ToLower(q.Get("keyword")))
	statusFilter := strings.TrimSpace(strings.ToLower(q.Get("status")))
	tlsFilter := strings.TrimSpace(strings.ToLower(q.Get("tls")))

	a.mu.RLock()
	certs := append([]model.CertStatus(nil), a.latest.Certificates...)
	proxies := append([]model.ProxyTraffic(nil), a.latest.Proxies...)
	a.mu.RUnlock()

	certProxyMap := make(map[string][]string)
	for _, p := range proxies {
		for _, d := range p.Domains {
			key := strings.ToLower(strings.TrimSpace(d))
			if key == "" {
				continue
			}
			certProxyMap[key] = append(certProxyMap[key], p.Name)
		}
	}
	for domain, names := range certProxyMap {
		certProxyMap[domain] = uniqueSortedStrings(names)
	}

	filtered := make([]model.CertStatus, 0, len(certs))
	for _, c := range certs {
		related := strings.Join(certProxyMap[strings.ToLower(strings.TrimSpace(c.Domain))], ", ")
		c.RelatedProxy = related
		if keyword != "" {
			if !strings.Contains(strings.ToLower(c.Domain), keyword) && !strings.Contains(strings.ToLower(related), keyword) {
				continue
			}
		}
		if statusFilter != "" && certificateStatus(c) != statusFilter {
			continue
		}
		if tlsFilter == "ok" && !c.TLSOK {
			continue
		}
		if tlsFilter == "fail" && c.TLSOK {
			continue
		}
		filtered = append(filtered, c)
	}

	sort.Slice(filtered, func(i, j int) bool {
		ci, cj := filtered[i], filtered[j]
		cmp := 0
		switch sortKey {
		case "domain":
			cmp = strings.Compare(ci.Domain, cj.Domain)
		case "expires_at":
			cmp = strings.Compare(ci.ExpiresAt, cj.ExpiresAt)
		default: // days
			di, dj := 99999, 99999
			if ci.DaysLeft != nil {
				di = *ci.DaysLeft
			}
			if cj.DaysLeft != nil {
				dj = *cj.DaysLeft
			}
			switch {
			case di < dj:
				cmp = -1
			case di > dj:
				cmp = 1
			default:
				cmp = strings.Compare(ci.Domain, cj.Domain)
			}
		}
		if order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})

	items, meta := paginateCertificates(filtered, page, pageSize)
	writeJSON(w, model.CertificateListResponse{Items: items, Meta: meta})
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func queryInt(v string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return def
	}
	return n
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func paginateProxies(items []model.ProxyTraffic, page, pageSize int) ([]model.ProxyTraffic, model.PagedMeta) {
	total := len(items)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	return items[start:end], model.PagedMeta{Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}
}

func paginateCertificates(items []model.CertStatus, page, pageSize int) ([]model.CertStatus, model.PagedMeta) {
	total := len(items)
	totalPages := 1
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	if page > totalPages {
		page = totalPages
	}
	start := (page - 1) * pageSize
	if start < 0 {
		start = 0
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	if start > end {
		start = end
	}
	return items[start:end], model.PagedMeta{Total: total, Page: page, PageSize: pageSize, TotalPages: totalPages}
}

func certificateStatus(c model.CertStatus) string {
	if !c.TLSOK {
		return "fail"
	}
	if c.TLSHasLocalCert && !c.TLSMatchLocal {
		return "fail"
	}
	if !c.Present || !c.OK {
		return "fail"
	}
	if c.DaysLeft != nil && *c.DaysLeft < 15 {
		return "warn"
	}
	return "ok"
}

func (a *App) handleDaily(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("每日流量查询请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := a.store.DailyTraffic()
	if err != nil {
		logger.Error("每日流量数据查询失败: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, data)
}

func (a *App) handleDailyInterface(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromDay := strings.TrimSpace(r.URL.Query().Get("from"))
	toDay := strings.TrimSpace(r.URL.Query().Get("to"))
	sortKey := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("sort")))
	if sortKey == "" {
		sortKey = "day"
	}
	order := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("order")))
	if order != "asc" {
		order = "desc"
	}
	data, err := a.store.DailyInterfaceTraffic(fromDay, toDay)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	settings := a.appcfg.PublicSettings()
	data = applyInitialTrafficToDailyRows(settings, data, fromDay, toDay)
	sortDailyInterfaceRows(data, sortKey, order)
	writeJSON(w, data)
}

func sortDailyInterfaceRows(rows []map[string]any, sortKey, order string) {
	sort.Slice(rows, func(i, j int) bool {
		ri, rj := rows[i], rows[j]
		cmp := 0
		switch sortKey {
		case "in", "rx", "rx_kb":
			cmp = compareUint64(numericAnyToUint64(ri["rx_kb"]), numericAnyToUint64(rj["rx_kb"]))
		case "out", "tx", "tx_kb":
			cmp = compareUint64(numericAnyToUint64(ri["tx_kb"]), numericAnyToUint64(rj["tx_kb"]))
		case "total":
			ti := numericAnyToUint64(ri["rx_kb"]) + numericAnyToUint64(ri["tx_kb"])
			tj := numericAnyToUint64(rj["rx_kb"]) + numericAnyToUint64(rj["tx_kb"])
			cmp = compareUint64(ti, tj)
		default:
			cmp = strings.Compare(fmt.Sprint(ri["day"]), fmt.Sprint(rj["day"]))
		}
		if cmp == 0 {
			cmp = strings.Compare(fmt.Sprint(ri["day"]), fmt.Sprint(rj["day"]))
		}
		if cmp == 0 {
			cmp = strings.Compare(fmt.Sprint(ri["iface"]), fmt.Sprint(rj["iface"]))
		}
		if cmp == 0 {
			cmp = strings.Compare(fmt.Sprint(ri["public_ip"]), fmt.Sprint(rj["public_ip"]))
		}
		if order == "asc" {
			return cmp < 0
		}
		return cmp > 0
	})
}

func compareUint64(a, b uint64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func applyInitialTrafficToMonth(settings model.PublicSettings, month string, inBytes, outBytes uint64) (uint64, uint64) {
	if !initialTrafficApplies(settings, month) {
		return inBytes, outBytes
	}
	return inBytes + mail.GBToBytes(settings.InitialInGB), outBytes + mail.GBToBytes(settings.InitialOutGB)
}

func applyInitialTrafficToDailyRows(settings model.PublicSettings, rows []map[string]any, fromDay, toDay string) []map[string]any {
	if settings.InitialInGB <= 0 && settings.InitialOutGB <= 0 {
		return rows
	}
	deployDay := strings.TrimSpace(settings.DeployDate)
	if !dateInRange(deployDay, fromDay, toDay) {
		return rows
	}
	initialRxKB := mail.GBToBytes(settings.InitialInGB) / 1024
	initialTxKB := mail.GBToBytes(settings.InitialOutGB) / 1024
	if initialRxKB == 0 && initialTxKB == 0 {
		return rows
	}
	if rows == nil {
		rows = []map[string]any{}
	}
	for _, row := range rows {
		if fmt.Sprint(row["day"]) != deployDay {
			continue
		}
		row["rx_kb"] = numericAnyToUint64(row["rx_kb"]) + initialRxKB
		row["tx_kb"] = numericAnyToUint64(row["tx_kb"]) + initialTxKB
		row["initial_adjusted"] = true
		return rows
	}
	rows = append(rows, map[string]any{
		"day":              deployDay,
		"iface":            "initial",
		"public_ip":        "initial",
		"rx_kb":            initialRxKB,
		"tx_kb":            initialTxKB,
		"initial_adjusted": true,
	})
	sort.Slice(rows, func(i, j int) bool {
		di, dj := fmt.Sprint(rows[i]["day"]), fmt.Sprint(rows[j]["day"])
		if di == dj {
			return fmt.Sprint(rows[i]["iface"]) < fmt.Sprint(rows[j]["iface"])
		}
		return di < dj
	})
	return rows
}

func initialTrafficApplies(settings model.PublicSettings, month string) bool {
	deployDay := strings.TrimSpace(settings.DeployDate)
	return len(deployDay) >= 7 && deployDay[:7] == month && (settings.InitialInGB > 0 || settings.InitialOutGB > 0)
}

func dateInRange(day, fromDay, toDay string) bool {
	if len(day) != 10 {
		return false
	}
	if strings.TrimSpace(fromDay) != "" && day < fromDay {
		return false
	}
	if strings.TrimSpace(toDay) != "" && day > toDay {
		return false
	}
	return true
}

func numericAnyToUint64(v any) uint64 {
	switch n := v.(type) {
	case uint64:
		return n
	case uint:
		return uint64(n)
	case int64:
		if n > 0 {
			return uint64(n)
		}
	case int:
		if n > 0 {
			return uint64(n)
		}
	case float64:
		if n > 0 {
			return uint64(n)
		}
	}
	return 0
}

func detectOutboundPublicIP() (string, error) {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 2*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || addr == nil || addr.IP == nil {
		return "", errors.New("invalid local udp address")
	}
	ip := addr.IP.To4()
	if ip == nil {
		return "", errors.New("non-ipv4 outbound ip")
	}
	return ip.String(), nil
}

func interfaceByIP(ipStr string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return "", errors.New("invalid ip")
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var addrIP net.IP
			switch a := addr.(type) {
			case *net.IPNet:
				addrIP = a.IP
			case *net.IPAddr:
				addrIP = a.IP
			}
			if addrIP == nil {
				continue
			}
			if addrIP.Equal(ip) {
				return iface.Name, nil
			}
		}
	}
	return "", errors.New("no interface bound with ip")
}

func readInterfaceCounters(statsDir, iface string) (uint64, uint64, error) {
	base := strings.TrimSpace(statsDir)
	if base == "" {
		base = "/host-net-stats"
	}
	// Backward compatible fallback: if mounted path is not available, read host sysfs directly.
	if _, err := os.Stat(base); err != nil {
		base = filepath.Join("/sys/class/net", iface, "statistics")
	}
	rxRaw, err := os.ReadFile(filepath.Join(base, "rx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	txRaw, err := os.ReadFile(filepath.Join(base, "tx_bytes"))
	if err != nil {
		return 0, 0, err
	}
	rx, err := strconv.ParseUint(strings.TrimSpace(string(rxRaw)), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	tx, err := strconv.ParseUint(strings.TrimSpace(string(txRaw)), 10, 64)
	if err != nil {
		return 0, 0, err
	}
	return rx, tx, nil
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := a.appcfg.PublicSettings()
		logger.Info("设置数据已读取 来源=%s", r.RemoteAddr)
		writeJSON(w, settings)
	case http.MethodPost:
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			logger.Warn("设置数据解析失败 来源=%s 错误=%v", r.RemoteAddr, err)
			http.Error(w, err.Error(), 400)
			return
		}
		allowed := map[string]bool{"threshold_in_gb": true, "threshold_out_gb": true, "threshold_total_gb": true, "limit_in_gb": true, "limit_out_gb": true, "limit_total_gb": true, "initial_in_gb": true, "initial_out_gb": true, "history_retention_days": true, "disk_free_space_alert_threshold_mb": true, "smtp_host": true, "smtp_port": true, "smtp_user": true, "smtp_auth_code": true, "smtp_from": true, "smtp_to": true, "smtp_enabled": true, "alert_proxy_offline": true, "alert_cert_expiry": true, "alert_cert_days": true}
		filtered := make(map[string]any)
		for key, value := range in {
			if !allowed[key] {
				logger.Warn("忽略未知设置项 键=%s 来源=%s", key, r.RemoteAddr)
				continue
			}
			filtered[key] = value
		}
		smtpChanged := a.appcfg.ApplyPOST(filtered)
		if smtpChanged {
			logger.Info("SMTP 配置已变更，验证状态已重置")
		}
		settings := a.appcfg.PublicSettings()
		a.syncSMTPWarnings(settings)
		logger.Info("设置已保存 项数=%d SMTP已变更=%t", len(in), smtpChanged)
		writeJSON(w, settings)
	default:
		logger.Warn("设置请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("测试邮件请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings := a.appcfg.PublicSettings()
	if err := mail.Send(settings, settings.SMTPAuthCode, "FRPS状态监控 - 测试邮件", "这是一封来自 FRPS状态监控 的测试邮件，SMTP 配置正常。"); err != nil {
		logger.Error("测试邮件发送失败: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	a.appcfg.SetSMTPVerified(true)
	settings = a.appcfg.PublicSettings()
	a.syncSMTPWarnings(settings)
	logger.Info("测试邮件已发送 收件人=%s", settings.SMTPTo)
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handleVacuum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("数据库整理请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := a.store.Vacuum(); err != nil {
		logger.Error("数据库整理失败: %v", err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	logger.Info("数据库整理完成")
	writeJSON(w, map[string]any{"ok": true})
}

func (a *App) handlePurge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("数据清理请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Days int `json:"days"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Days < 1 {
		body.Days = 60
	}
	a.appcfg.SetHistoryRetentionDays(body.Days)
	deleted, err := a.store.Purge(body.Days)
	if err != nil {
		logger.Error("清理流量数据失败 保留天数=%d 错误=%v", body.Days, err)
		writeJSON(w, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	logger.Info("流量数据清理完成 保留天数=%d 已删除=%d", body.Days, deleted)
	writeJSON(w, map[string]any{"ok": true, "deleted": deleted})
}

func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("导出 CSV 请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := a.store.DailyTraffic()
	if err != nil {
		logger.Error("导出 CSV 时查询流量数据失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="frps-traffic.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"day", "proxy_name", "proxy_type", "in_bytes", "out_bytes", "peak_conns"})
	for _, row := range rows {
		_ = cw.Write([]string{
			fmt.Sprint(row["day"]),
			fmt.Sprint(row["name"]),
			fmt.Sprint(row["type"]),
			strconv.FormatInt(int64(row["in"].(int64)), 10),
			strconv.FormatInt(int64(row["out"].(int64)), 10),
			strconv.FormatInt(int64(row["peak_conns"].(int64)), 10),
		})
	}
	cw.Flush()
	logger.Info("流量数据 CSV 已导出 行数=%d 来源=%s", len(rows), r.RemoteAddr)
}

func (a *App) handleCurrentLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("查看当前日志请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path, content, size, err := logger.CurrentLog(256 * 1024)
	if err != nil {
		logger.Error("读取当前日志失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"path":    path,
		"size":    size,
		"content": content,
	})
}

func (a *App) handleClearLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("清空日志请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := logger.ClearCurrentLog(); err != nil {
		logger.Error("清空当前日志失败: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	logger.Warn("当前日志已清空 来源=%s", r.RemoteAddr)
	writeJSON(w, map[string]any{"ok": true})
}

// ── credential validation ────────────────────────────────────────────────────

const allowedSpecial = "*@#!()-_"

func isAlphaRune(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isDigitRune(r rune) bool { return r >= '0' && r <= '9' }
func toLowerRune(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + 32
	}
	return r
}

func validatePassword(password string) error {
	if len(password) <= 8 || len(password) >= 32 {
		logger.Warn("密码验证失败：密码长度必须大于 8 位且小于 32 位")
		return errors.New("密码长度必须大于 8 位且小于 32 位")
	}
	hasLetter, hasDigit := false, false
	for _, r := range password {
		switch {
		case isAlphaRune(r):
			hasLetter = true
		case isDigitRune(r):
			hasDigit = true
		case strings.ContainsRune(allowedSpecial, r):
			// ok
		default:
			logger.Warn("密码验证失败：密码只能包含英文字母、数字及 *@#!()-_ 特殊字符")
			return errors.New("密码只能包含英文字母、数字及 *@#!()-_ 特殊字符")
		}
	}
	if !hasLetter {
		logger.Warn("密码验证失败：密码必须包含英文字母")
		return errors.New("密码必须包含英文字母")
	}
	if !hasDigit {
		logger.Warn("密码验证失败：密码必须包含数字")
		return errors.New("密码必须包含数字")
	}
	runes := []rune(password)
	for i := 2; i < len(runes); i++ {
		a, b, c := runes[i-2], runes[i-1], runes[i]
		if a == b && b == c {
			logger.Warn("密码验证失败：密码不能包含 3 个及以上相同连续字符（如 aaa、111）")
			return errors.New("密码不能包含 3 个及以上相同连续字符（如 aaa、111）")
		}
		al, bl, cl := toLowerRune(a), toLowerRune(b), toLowerRune(c)
		if (isAlphaRune(a) && isAlphaRune(b) && isAlphaRune(c)) ||
			(isDigitRune(a) && isDigitRune(b) && isDigitRune(c)) {
			if (bl == al+1 && cl == bl+1) || (bl == al-1 && cl == bl-1) {
				logger.Warn("密码验证失败：密码不能包含 3 个及以上连续递增或递减字符（如 abc、123）")
				return errors.New("密码不能包含 3 个及以上连续递增或递减字符（如 abc、123）")
			}
		}
	}
	return nil
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		logger.Warn("用户名验证失败：用户名长度必须在 3 到 32 位之间")
		return errors.New("用户名长度必须在 3 到 32 位之间")
	}
	for _, r := range username {
		switch {
		case isAlphaRune(r), isDigitRune(r):
		case strings.ContainsRune(allowedSpecial, r):
		default:
			logger.Warn("用户名验证失败：用户名只能包含英文字母、数字及 *@#!()-_ 特殊字符")
			return errors.New("用户名只能包含英文字母、数字及 *@#!()-_ 特殊字符")
		}
	}
	return nil
}

// ── warning helpers ─────────────────────────────────────────────────────────

func (a *App) InitWarnings() {
	settings := a.appcfg.PublicSettings()
	a.syncSMTPWarnings(settings)
	a.syncRecoveryEmailWarning()
}

func (a *App) syncSMTPWarnings(settings model.PublicSettings) {
	configured := settings.SMTPHost != "" && settings.SMTPFrom != "" &&
		settings.SMTPTo != "" && settings.SMTPAuthCode != ""
	verified := a.appcfg.SMTPVerified()
	if !configured {
		_ = a.store.SetWarning("smtp_not_configured", "SMTP 邮件未配置，将无法收到任何告警通知")
		_ = a.store.ClearWarning("smtp_not_verified")
		return
	}
	if !verified {
		_ = a.store.SetWarning("smtp_not_configured", "SMTP 邮件尚未验证，请在“配置邮件”中发送测试邮件完成验证")
		_ = a.store.SetWarning("smtp_not_verified", "SMTP 邮件配置未验证，请发送测试邮件确认配置正确")
		return
	}
	_ = a.store.ClearWarning("smtp_not_configured")
	_ = a.store.ClearWarning("smtp_not_verified")
}

func (a *App) syncRecoveryEmailWarning() {
	u, err := a.store.GetUser()
	if err != nil || u.RecoveryEmail == "" {
		_ = a.store.SetWarning("user_no_recovery_email", "未设置密码找回邮箱，忘记密码时将无法重置账户凭据")
	} else {
		_ = a.store.ClearWarning("user_no_recovery_email")
	}
}

func (a *App) syncTrafficWarnings(settings model.PublicSettings, monthIn, monthOut uint64) {
	thresholdInEnabled := settings.ThresholdInGB > 0
	thresholdOutEnabled := settings.ThresholdOutGB > 0
	thresholdTotalEnabled := settings.ThresholdTotalGB > 0 && thresholdInEnabled && thresholdOutEnabled
	limitInEnabled := settings.LimitInGB > 0
	limitOutEnabled := settings.LimitOutGB > 0
	limitTotalEnabled := settings.LimitTotalGB > 0 && limitInEnabled && limitOutEnabled
	total := monthIn + monthOut
	if thresholdInEnabled && monthIn >= mail.GBToBytes(settings.ThresholdInGB) {
		msg := fmt.Sprintf("本月网卡入站流量已超出阈值（当前：%s，阈值：%.2f GB）", mail.HumanBytes(monthIn), settings.ThresholdInGB)
		_ = a.store.SetWarning("traffic_in", msg)
	} else {
		_ = a.store.ClearWarning("traffic_in")
	}
	if thresholdOutEnabled && monthOut >= mail.GBToBytes(settings.ThresholdOutGB) {
		msg := fmt.Sprintf("本月网卡出站流量已超出阈值（当前：%s，阈值：%.2f GB）", mail.HumanBytes(monthOut), settings.ThresholdOutGB)
		_ = a.store.SetWarning("traffic_out", msg)
	} else {
		_ = a.store.ClearWarning("traffic_out")
	}
	if thresholdTotalEnabled && total >= mail.GBToBytes(settings.ThresholdTotalGB) {
		msg := fmt.Sprintf("本月网卡总流量已超出阈值（当前：%s，阈值：%.2f GB）", mail.HumanBytes(total), settings.ThresholdTotalGB)
		_ = a.store.SetWarning("traffic_total", msg)
	} else {
		_ = a.store.ClearWarning("traffic_total")
	}
	if limitInEnabled && monthIn >= mail.GBToBytes(settings.LimitInGB) {
		msg := fmt.Sprintf("本月网卡入站流量已达到限额（当前：%s，限额：%.2f GB）", mail.HumanBytes(monthIn), settings.LimitInGB)
		_ = a.store.SetWarning("traffic_limit_in", msg)
	} else {
		_ = a.store.ClearWarning("traffic_limit_in")
	}
	if limitOutEnabled && monthOut >= mail.GBToBytes(settings.LimitOutGB) {
		msg := fmt.Sprintf("本月网卡出站流量已达到限额（当前：%s，限额：%.2f GB）", mail.HumanBytes(monthOut), settings.LimitOutGB)
		_ = a.store.SetWarning("traffic_limit_out", msg)
	} else {
		_ = a.store.ClearWarning("traffic_limit_out")
	}
	if limitTotalEnabled && total >= mail.GBToBytes(settings.LimitTotalGB) {
		msg := fmt.Sprintf("本月网卡总流量已达到限额（当前：%s，限额：%.2f GB）", mail.HumanBytes(total), settings.LimitTotalGB)
		_ = a.store.SetWarning("traffic_limit_total", msg)
	} else {
		_ = a.store.ClearWarning("traffic_limit_total")
	}
}

func (a *App) handleWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	warnings, err := a.store.GetWarnings()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, warnings)
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
