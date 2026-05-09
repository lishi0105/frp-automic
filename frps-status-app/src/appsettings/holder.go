package appsettings

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"frps-status-app.local/status/src/model"
	"frps-status-app.local/status/src/store"
)

// state 为进程内可变应用配置（与 model.PublicSettings 对齐，并含 smtp_verified）。
type state struct {
	DeployDate                    string
	ThresholdInGB                 float64
	ThresholdOutGB                float64
	ThresholdTotalGB              float64
	LimitInGB                     float64
	LimitOutGB                    float64
	LimitTotalGB                  float64
	InitialInGB                   float64
	InitialOutGB                  float64
	HistoryRetentionDays          int
	DiskFreeSpaceAlertThresholdMB uint64
	SMTPHost                      string
	SMTPPort                      int
	SMTPUser                      string
	SMTPFrom                      string
	SMTPTo                        string
	SMTPEnabled                   bool
	SMTPAuthCode                  string
	SMTPVerified                  bool
	AlertProxyOffline             bool
	AlertCertExpiry               bool
	AlertCertDays                 int
}

func defaultState() state {
	return state{
		HistoryRetentionDays:          60,
		DiskFreeSpaceAlertThresholdMB: 0,
		SMTPPort:                      465,
		AlertCertDays:                 15,
	}
}

func (s *state) normalize() {
	if s.HistoryRetentionDays < 1 {
		s.HistoryRetentionDays = 60
	}
	if s.HistoryRetentionDays > 365 {
		s.HistoryRetentionDays = 365
	}
	if s.AlertCertDays < 1 {
		s.AlertCertDays = 15
	}
	if s.AlertCertDays > 90 {
		s.AlertCertDays = 90
	}
	if s.SMTPPort < 1 || s.SMTPPort > 65535 {
		s.SMTPPort = 465
	}
}

func (s state) toPublic() model.PublicSettings {
	return model.PublicSettings{
		DeployDate:                    s.DeployDate,
		ThresholdInGB:                 s.ThresholdInGB,
		ThresholdOutGB:                s.ThresholdOutGB,
		ThresholdTotalGB:              s.ThresholdTotalGB,
		LimitInGB:                     s.LimitInGB,
		LimitOutGB:                    s.LimitOutGB,
		LimitTotalGB:                  s.LimitTotalGB,
		InitialInGB:                   s.InitialInGB,
		InitialOutGB:                  s.InitialOutGB,
		HistoryRetentionDays:          s.HistoryRetentionDays,
		DiskFreeSpaceAlertThresholdMB: s.DiskFreeSpaceAlertThresholdMB,
		SMTPHost:                      s.SMTPHost,
		SMTPPort:                      s.SMTPPort,
		SMTPUser:                      s.SMTPUser,
		SMTPFrom:                      s.SMTPFrom,
		SMTPTo:                        s.SMTPTo,
		SMTPEnabled:                   s.SMTPEnabled,
		SMTPAuthCode:                  s.SMTPAuthCode,
		AlertProxyOffline:             s.AlertProxyOffline,
		AlertCertExpiry:               s.AlertCertExpiry,
		AlertCertDays:                 s.AlertCertDays,
	}
}

func parseFloat(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
		return 0
	}
	return f
}

func parseFloatDefault(v string, fallback float64) float64 {
	f := parseFloat(v)
	if f == 0 {
		return fallback
	}
	return f
}

func parseDiskAlertThresholdMB(v string) uint64 {
	f := parseFloat(v)
	if f <= 0 || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0
	}
	return uint64(f + 0.5)
}

// Manager 线程安全的进程内应用配置（内置默认值，不写库、不写文件；重启后恢复默认）。
// 部署日期 DeployDate 仅来自数据库 app_meta，与 DB 文件生命周期一致。
type Manager struct {
	mu   sync.RWMutex
	data state
	st   *store.Store
}

func New(st *store.Store) *Manager {
	m := &Manager{data: defaultState(), st: st}
	m.data.normalize()
	return m
}

func (m *Manager) PublicSettings() model.PublicSettings {
	m.mu.RLock()
	out := m.data.toPublic()
	m.mu.RUnlock()
	if m.st != nil {
		if d := strings.TrimSpace(m.st.DeployDate()); d != "" {
			out.DeployDate = d
			return out
		}
	}
	if strings.TrimSpace(out.DeployDate) == "" {
		out.DeployDate = time.Now().Format("2006-01-02")
	}
	return out
}

func (m *Manager) SMTPVerified() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.data.SMTPVerified
}

func (m *Manager) SetDiskFreeSpaceAlertThresholdMB(mb uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data.DiskFreeSpaceAlertThresholdMB = mb
	m.data.normalize()
}

func (m *Manager) SetSMTPVerified(v bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data.SMTPVerified = v
	m.data.normalize()
}

func (m *Manager) SetHistoryRetentionDays(days int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data.HistoryRetentionDays = days
	m.data.normalize()
}

// ApplyPOST 合并前端 POST 的字段，仅更新内存。
func (m *Manager) ApplyPOST(in map[string]any) (smtpChanged bool) {
	smtpKeys := map[string]bool{
		"smtp_host": true, "smtp_port": true, "smtp_user": true, "smtp_auth_code": true,
		"smtp_from": true, "smtp_to": true, "smtp_enabled": true,
	}
	for k := range in {
		if smtpKeys[k] {
			smtpChanged = true
			break
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	d := &m.data
	for key, value := range in {
		s := fmt.Sprint(value)
		switch key {
		case "threshold_in_gb":
			d.ThresholdInGB = parseFloat(s)
		case "threshold_out_gb":
			d.ThresholdOutGB = parseFloat(s)
		case "threshold_total_gb":
			d.ThresholdTotalGB = parseFloat(s)
		case "limit_in_gb":
			d.LimitInGB = parseFloat(s)
		case "limit_out_gb":
			d.LimitOutGB = parseFloat(s)
		case "limit_total_gb":
			d.LimitTotalGB = parseFloat(s)
		case "initial_in_gb":
			d.InitialInGB = parseFloat(s)
		case "initial_out_gb":
			d.InitialOutGB = parseFloat(s)
		case "history_retention_days":
			d.HistoryRetentionDays = int(parseFloatDefault(s, 60))
		case "disk_free_space_alert_threshold_mb":
			d.DiskFreeSpaceAlertThresholdMB = parseDiskAlertThresholdMB(s)
		case "smtp_host":
			d.SMTPHost = s
		case "smtp_port":
			d.SMTPPort = int(parseFloatDefault(s, 465))
		case "smtp_user":
			d.SMTPUser = s
		case "smtp_auth_code":
			d.SMTPAuthCode = s
		case "smtp_from":
			d.SMTPFrom = s
		case "smtp_to":
			d.SMTPTo = s
		case "smtp_enabled":
			d.SMTPEnabled = strings.EqualFold(s, "true")
		case "alert_proxy_offline":
			d.AlertProxyOffline = strings.EqualFold(s, "true")
		case "alert_cert_expiry":
			d.AlertCertExpiry = strings.EqualFold(s, "true")
		case "alert_cert_days":
			d.AlertCertDays = int(parseFloatDefault(s, 15))
		default:
		}
	}
	if smtpChanged {
		d.SMTPVerified = false
	}
	d.normalize()
	return smtpChanged
}
