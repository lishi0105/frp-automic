package alerting

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/mail"
	"frps-status-app.local/status/src/model"
	"frps-status-app.local/status/src/monitor"
	"frps-status-app.local/status/src/store"
)

const (
	// 父级 parent_key，与表 alert_parent_definitions.parent_key、告警 fingerprint 一致
	pkMonitorNetwork = "monitor_network_down"
	pkHostCPU        = "host_cpu_saturation"
	pkHostMem        = "host_memory_pressure"
	pkHostDisk       = "host_storage_pressure"
	pkFrpsDashboard  = "frps_dashboard_unreachable"

	defaultParentRank = 999

	proxyConfirmFailures = 3
	certConfirmFailures  = 2

	globalMinFailures  = 5
	globalFailRatioPct = 30

	proxyCooldown        = 30 * time.Minute
	certCooldown         = 6 * time.Hour
	globalCooldown       = 30 * time.Minute
	parentCooldown       = 30 * time.Minute
	flapCooldown         = 30 * time.Minute
	trafficCooldown      = 32 * 24 * time.Hour
	diskCriticalCooldown = 24 * time.Hour

	flapWindow        = 10 * time.Minute
	flapSwitchesLimit = 3

	globalCandidateWindow = 2 * time.Minute
)

type Manager struct {
	store *store.Store
}

type candidate struct {
	Fingerprint  string
	DefinitionID string // alert_parent_definitions.id（UUID），非父级告警可空
	AlertType    string
	Target       string
	Level        string
	Title        string
	Message      string
	LastError    string
	Cooldown     time.Duration
	WarningKey   string
}

type parentActivation struct {
	key  string
	def  store.AlertParentDef
	rank int
}

func New(st *store.Store) *Manager {
	return &Manager{store: st}
}

func (m *Manager) parentRecoveryConfirmed(key string, healthy bool, now time.Time) bool {
	st, err := m.store.GetEventState(key)
	if err != nil {
		logger.Error("读取父级恢复状态失败 key=%s 错误=%v", key, err)
		return false
	}
	if !healthy {
		st.Key = key
		st.FailStreak++
		if st.FirstFailAt == "" {
			st.FirstFailAt = now.Format(time.RFC3339)
		}
		st.RecoverStreak = 0
		st.RecoverSince = ""
		st.LastSeenAt = now.Format(time.RFC3339)
		if err := m.store.SaveEventState(st); err != nil {
			logger.Error("保存父级异常状态失败 key=%s 错误=%v", key, err)
		}
		return false
	}
	st.Key = key
	st.FailStreak = 0
	st.FirstFailAt = ""
	st.RecoverStreak++
	if st.RecoverSince == "" {
		st.RecoverSince = now.Format(time.RFC3339)
	}
	st.LastSeenAt = now.Format(time.RFC3339)
	if err := m.store.SaveEventState(st); err != nil {
		logger.Error("保存父级恢复状态失败 key=%s 错误=%v", key, err)
		return false
	}
	return st.RecoveryConfirmed(now)
}

func (m *Manager) Process(settings model.PublicSettings, result monitor.Result) {
	defs := m.store.AlertParentDefsByKey()
	m.processTraffic(settings, result.Traffic, defs)

	// 条件已恢复则先发父级恢复（与本轮「谁触发」无关）
	if result.LinkChainOK() {
		m.resolve(settings, pkMonitorNetwork, "检测链路恢复", "DNS 与外网 HTTPS 探测已恢复。代理与证书告警将按最新检测结果重新判断。")
		_ = m.store.ClearWarning(pkMonitorNetwork)
	}
	if result.LinkChainOK() && result.ProxyFetchError == "" {
		m.resolve(settings, pkFrpsDashboard, "FRPS 面板检测恢复", "FRPS 面板已恢复可访问，代理与证书告警已重新按最新检测结果判断。")
		_ = m.store.ClearWarning(pkFrpsDashboard)
	}

	now := time.Now().UTC()
	hp := result.HostPressure
	if m.parentRecoveryConfirmed(pkHostCPU, !hp.CPUOverload, now) {
		m.resolve(settings, pkHostCPU, "CPU 负载恢复", "最近 1 分钟平均负载已回落至 80% 阈值以下（按 CPU 数折算）。代理与证书告警将按最新检测结果重新判断。")
		_ = m.store.ClearWarning(pkHostCPU)
	}
	if m.parentRecoveryConfirmed(pkHostMem, !hp.MemLow, now) {
		m.resolve(settings, pkHostMem, "内存压力恢复", "可用内存占比已回到 10% 阈值以上。代理与证书告警将按最新检测结果重新判断。")
		_ = m.store.ClearWarning(pkHostMem)
	}
	if m.parentRecoveryConfirmed(pkHostDisk, !hp.DiskLow, now) {
		m.resolve(settings, pkHostDisk, "磁盘空间压力恢复", "数据目录所在分区剩余空间比例已回到 10% 阈值以上。代理与证书告警将按最新检测结果重新判断。")
		_ = m.store.ClearWarning(pkHostDisk)
	}

	proxySuppressed, parentEarlyExit := m.applyParentFaults(settings, result, defs)
	if parentEarlyExit {
		return
	}

	var proxyAlerts []candidate
	if settings.AlertProxyOffline && !proxySuppressed {
		proxyAlerts = m.proxyCandidates(result.Proxies)
	}
	var certAlerts []candidate
	if settings.AlertCertExpiry && !proxySuppressed {
		certAlerts = m.certCandidates(result.Certificates)
	}
	allAlerts := append(append([]candidate{}, proxyAlerts...), certAlerts...)

	if m.processGlobal(settings, allAlerts, len(result.Proxies)+len(result.Certificates)) {
		return
	}

	m.resolve(settings, "global_monitor_fault", "FRPS 全局故障恢复", "此前触发的批量异常已恢复，系统恢复为单项告警判断。")
	_ = m.store.ClearWarning("global_monitor_fault")

	for _, c := range proxyAlerts {
		m.fire(settings, c)
	}
	for _, c := range certAlerts {
		m.fire(settings, c)
	}
	if settings.AlertProxyOffline && !proxySuppressed {
		m.resolveMissingProxyAlerts(settings, result.Proxies)
	}
	if settings.AlertCertExpiry && !proxySuppressed {
		m.resolveMissingCertAlerts(settings, result.Certificates)
	}
}

func lookupParentDef(defs map[string]store.AlertParentDef, key string) store.AlertParentDef {
	d, ok := defs[key]
	if !ok {
		return store.AlertParentDef{ParentKey: key, SortRank: defaultParentRank, SuppressProxyCert: true}
	}
	return d
}

func parentRank(d store.AlertParentDef) int {
	if d.SortRank <= 0 {
		return defaultParentRank
	}
	return d.SortRank
}

// applyParentFaults 按 alert_parent_definitions.sort_rank 只执行本轮最小 rank 的父级（同秩可同时执行）；更高 rank 的已触发父级仅打日志不 fire。
func (m *Manager) applyParentFaults(settings model.PublicSettings, result monitor.Result, defs map[string]store.AlertParentDef) (proxySuppressed bool, earlyExit bool) {
	var acts []parentActivation
	if !result.LinkChainOK() {
		d := lookupParentDef(defs, pkMonitorNetwork)
		acts = append(acts, parentActivation{key: pkMonitorNetwork, def: d, rank: parentRank(d)})
	}
	if result.HostPressure.CPUOverload {
		d := lookupParentDef(defs, pkHostCPU)
		acts = append(acts, parentActivation{key: pkHostCPU, def: d, rank: parentRank(d)})
	}
	if result.HostPressure.MemLow {
		d := lookupParentDef(defs, pkHostMem)
		acts = append(acts, parentActivation{key: pkHostMem, def: d, rank: parentRank(d)})
	}
	if result.HostPressure.DiskLow {
		d := lookupParentDef(defs, pkHostDisk)
		acts = append(acts, parentActivation{key: pkHostDisk, def: d, rank: parentRank(d)})
	}
	if result.LinkChainOK() && result.ProxyFetchError != "" {
		d := lookupParentDef(defs, pkFrpsDashboard)
		acts = append(acts, parentActivation{key: pkFrpsDashboard, def: d, rank: parentRank(d)})
	}
	if len(acts) == 0 {
		return false, false
	}
	minR := acts[0].rank
	for _, a := range acts[1:] {
		if a.rank < minR {
			minR = a.rank
		}
	}
	var winners []parentActivation
	for _, a := range acts {
		if a.rank == minR {
			winners = append(winners, a)
		} else {
			logger.Info("父级告警本轮被更高优先级抑制 parent_key=%s sort_rank=%d winner_min_rank=%d", a.key, a.rank, minR)
		}
	}
	sort.Slice(winners, func(i, j int) bool { return winners[i].key < winners[j].key })

	for _, w := range winners {
		switch w.key {
		case pkMonitorNetwork:
			detail := monitor.LinkHealthSummary(result.LinkHealth)
			if result.ProxyFetchError != "" {
				detail += "\n\n附注：本轮 FRPS 代理列表拉取亦失败（可能与网络同源）。错误：" + result.ProxyFetchError
			}
			m.fire(settings, candidate{
				Fingerprint:  pkMonitorNetwork,
				DefinitionID: w.def.ID,
				AlertType:    "parent_fault",
				Target:       "monitor_host",
				Level:        "critical",
				Title:        "检测服务器网络或外网不可用",
				Message:      detail,
				LastError:    detail,
				Cooldown:     parentCooldown,
				WarningKey:   pkMonitorNetwork,
			})
			m.suppressProxyChildren(result.Proxies, pkMonitorNetwork, "检测链路异常，代理状态不可判断")
			m.suppressCertChildren(result.Certificates, pkMonitorNetwork, "检测链路异常，证书在线检测结果不可信")
		case pkHostCPU:
			hp := result.HostPressure
			ratioPct := 0.0
			if hp.OnlineCPUs > 0 {
				ratioPct = 100.0 * hp.Load1 / float64(hp.OnlineCPUs)
			}
			msg := fmt.Sprintf("最近 1 分钟平均负载 %.2f，按 %d 个 CPU 折算负载率 %.0f%%，已超过 80%% 阈值（load/ncpu）。数据来源：/proc/loadavg。", hp.Load1, hp.OnlineCPUs, ratioPct)
			if len(hp.ProbeErrors) > 0 {
				msg += "\n\n探测附注：" + strings.Join(hp.ProbeErrors, "；")
			}
			m.fire(settings, candidate{
				Fingerprint:  pkHostCPU,
				DefinitionID: w.def.ID,
				AlertType:    "parent_fault",
				Target:       "host_cpu",
				Level:        "critical",
				Title:        "FRPS 状态机 — CPU 负载过高",
				Message:      msg,
				LastError:    msg,
				Cooldown:     parentCooldown,
				WarningKey:   pkHostCPU,
			})
			m.suppressProxyChildren(result.Proxies, pkHostCPU, "本机 CPU 负载过高，代理在线结论可能失真")
			m.suppressCertChildren(result.Certificates, pkHostCPU, "本机 CPU 负载过高，证书检测结论可能失真")
		case pkHostMem:
			hp := result.HostPressure
			msg := fmt.Sprintf("可用内存占比约 %.1f%%（MemAvailable/MemTotal，可用约 %s / 总计约 %s），低于 10%% 阈值。数据来源：/proc/meminfo。",
				hp.MemAvailRatio*100, mail.HumanBytes(hp.MemAvailBytes), mail.HumanBytes(hp.MemTotalBytes))
			if len(hp.ProbeErrors) > 0 {
				msg += "\n\n探测附注：" + strings.Join(hp.ProbeErrors, "；")
			}
			m.fire(settings, candidate{
				Fingerprint:  pkHostMem,
				DefinitionID: w.def.ID,
				AlertType:    "parent_fault",
				Target:       "host_memory",
				Level:        "critical",
				Title:        "FRPS 状态机 — 可用内存不足",
				Message:      msg,
				LastError:    msg,
				Cooldown:     parentCooldown,
				WarningKey:   pkHostMem,
			})
			m.suppressProxyChildren(result.Proxies, pkHostMem, "本机可用内存过低，代理在线结论可能失真")
			m.suppressCertChildren(result.Certificates, pkHostMem, "本机可用内存过低，证书检测结论可能失真")
		case pkHostDisk:
			hp := result.HostPressure
			msg := fmt.Sprintf("数据目录所在分区剩余空间比例 %.1f%%（剩余 %s / 总计 %s），低于 10%% 阈值。统计方式与存储页一致（statfs）。",
				hp.DiskFreeRatio*100, mail.HumanBytes(hp.DiskFreeBytes), mail.HumanBytes(hp.DiskTotalBytes))
			if len(hp.ProbeErrors) > 0 {
				msg += "\n\n探测附注：" + strings.Join(hp.ProbeErrors, "；")
			}
			m.fire(settings, candidate{
				Fingerprint:  pkHostDisk,
				DefinitionID: w.def.ID,
				AlertType:    "parent_fault",
				Target:       "host_disk",
				Level:        "critical",
				Title:        "FRPS 状态机 — 磁盘剩余空间过低",
				Message:      msg,
				LastError:    msg,
				Cooldown:     parentCooldown,
				WarningKey:   pkHostDisk,
			})
			m.suppressProxyChildren(result.Proxies, pkHostDisk, "本机磁盘剩余空间过低，代理与证书检测结论可能失真")
			m.suppressCertChildren(result.Certificates, pkHostDisk, "本机磁盘剩余空间过低，证书检测结论可能失真")
		case pkFrpsDashboard:
			m.fire(settings, candidate{
				Fingerprint:  pkFrpsDashboard,
				DefinitionID: w.def.ID,
				AlertType:    "parent_fault",
				Target:       "frps_dashboard",
				Level:        "critical",
				Title:        "FRPS 面板检测失败",
				Message:      fmt.Sprintf("无法获取 FRPS 代理列表：%s。代理离线与 SSL 证书检测类子告警已抑制，待面板恢复后重新判断。", result.ProxyFetchError),
				LastError:    result.ProxyFetchError,
				Cooldown:     parentCooldown,
				WarningKey:   pkFrpsDashboard,
			})
			m.suppressProxyChildren(result.Proxies, pkFrpsDashboard, "FRPS 面板不可达，代理状态不可判断")
			m.suppressCertChildren(result.Certificates, pkFrpsDashboard, "FRPS 面板不可达，公网证书握手结论不可单独采信")
		default:
			logger.Warn("父级 parent_key=%s 已触发但尚未接入执行逻辑", w.key)
		}
	}

	for _, w := range winners {
		if w.def.SuppressProxyCert {
			proxySuppressed = true
		}
		if w.def.BlocksDownstream {
			earlyExit = true
		}
	}
	return proxySuppressed, earlyExit
}

func (m *Manager) processTraffic(settings model.PublicSettings, traffic monitor.TrafficResult, defs map[string]store.AlertParentDef) {
	for _, item := range append(append([]monitor.TrafficThreshold{}, traffic.Thresholds...), traffic.LimitReached...) {
		fp := item.Fingerprint + ":" + traffic.Month
		title := fmt.Sprintf("FRPS 网卡%s流量告警 %s", item.Label, traffic.Month)
		msg := fmt.Sprintf("FRPS 本月网卡%s流量为 %s，阈值为 %.2f GB。", item.Label, mail.HumanBytes(item.Current), item.ThresholdGB)
		d := lookupParentDef(defs, item.Fingerprint)
		m.fire(settings, candidate{
			Fingerprint:  fp,
			DefinitionID: d.ID,
			AlertType:    "traffic",
			Target:       item.Fingerprint,
			Level:        "warning",
			Title:        title,
			Message:      msg,
			Cooldown:     trafficCooldown,
		})
	}
}

func (m *Manager) FireDiskCritical(settings model.PublicSettings, message string) {
	m.fire(settings, candidate{
		Fingerprint: "disk_space_critical",
		AlertType:   "host_disk_critical",
		Target:      "data_partition",
		Level:       "critical",
		Title:       "FRPS 状态监控 - 磁盘空间严重不足告警",
		Message:     message + "\n\n（本邮件在 24 小时内最多发送一次。）",
		LastError:   message,
		Cooldown:    diskCriticalCooldown,
		WarningKey:  "disk_space_critical",
	})
}

func (m *Manager) ResolveDiskCritical(settings model.PublicSettings) {
	m.resolve(settings, "disk_space_critical", "磁盘空间严重不足恢复", "数据目录所在分区剩余空间已恢复至配置阈值以上。")
	_ = m.store.ClearWarning("disk_space_critical")
}

func (m *Manager) proxyCandidates(proxies []model.ProxyTraffic) []candidate {
	out := make([]candidate, 0)
	for _, p := range proxies {
		key := "proxy_offline:" + p.Type + ":" + p.Name
		flaps, _ := m.store.ProxyFlapCountSince(key, time.Now().Add(-flapWindow))
		if flaps >= flapSwitchesLimit {
			msg := fmt.Sprintf("代理 %s（类型：%s）在 10 分钟内状态切换 %d 次，已判定为状态抖动。普通离线/恢复通知已抑制。", p.Name, p.Type, flaps)
			out = append(out, candidate{
				Fingerprint: "proxy_flapping:" + p.Type + ":" + p.Name,
				AlertType:   "proxy_flapping",
				Target:      p.Name,
				Level:       "warning",
				Title:       fmt.Sprintf("FRPS 代理状态抖动 - %s", p.Name),
				Message:     msg,
				LastError:   msg,
				Cooldown:    flapCooldown,
				WarningKey:  "proxy_flapping:" + p.Type + ":" + p.Name,
			})
			_ = m.store.ClearWarning(key)
			continue
		}
		_ = m.store.ClearWarning("proxy_flapping:" + p.Type + ":" + p.Name)
		if p.Online || p.Health.ConsecutiveOffline < proxyConfirmFailures {
			continue
		}
		msg := fmt.Sprintf("代理 %s（类型：%s）连续 %d 次轮询离线，离线时长约 %d 秒，请检查客户端连接状态。", p.Name, p.Type, p.Health.ConsecutiveOffline, p.Health.OfflineSeconds)
		out = append(out, candidate{
			Fingerprint: key,
			AlertType:   "proxy_offline",
			Target:      p.Name,
			Level:       "warning",
			Title:       fmt.Sprintf("FRPS 代理离线告警 - %s", p.Name),
			Message:     msg,
			LastError:   msg,
			Cooldown:    proxyCooldown,
			WarningKey:  key,
		})
	}
	return out
}

func (m *Manager) certCandidates(certs []model.CertStatus) []candidate {
	out := make([]candidate, 0)
	now := time.Now().UTC().Format(time.RFC3339)
	for _, c := range certs {
		legacyKey := "cert_expiry:" + strings.TrimSpace(c.Domain)
		if legacyKey != "cert_expiry:" {
			_, _, _ = m.store.ResolveAlertEvent(legacyKey, fmt.Sprintf("FRPS SSL证书告警键已迁移 - %s", c.Domain), "证书告警已迁移为分阶段有效期 fingerprint 与 TLS 在线检测 fingerprint，旧聚合键已关闭。")
			_ = m.store.ClearWarning(legacyKey)
		}
		issues := certIssues(c)
		active := make(map[string]certIssue, len(issues))
		hasExpiryIssue := false
		for _, issue := range issues {
			active[issue.Fingerprint] = issue
			if strings.HasPrefix(issue.Fingerprint, "cert_expiry:") {
				hasExpiryIssue = true
			}
		}
		for _, key := range certKnownKeys(c.Domain) {
			issue, hasIssue := active[key]
			if !hasIssue && hasExpiryIssue && strings.HasPrefix(key, "cert_expiry:") {
				_, _, _ = m.store.ResolveAlertEvent(key, fmt.Sprintf("FRPS SSL证书告警阶段更新 - %s", c.Domain), "证书有效期告警已进入新的提醒阶段，旧阶段告警已关闭。")
				_ = m.store.ClearWarning(key)
				m.clearRecoverProgress(key)
				continue
			}
			st, err := m.store.GetEventState(key)
			if err != nil {
				continue
			}
			prevFail := st.FailStreak
			if hasIssue {
				st.RecoverStreak = 0
				st.RecoverSince = ""
				st.FailStreak++
				if st.FirstFailAt == "" {
					st.FirstFailAt = now
				}
			} else {
				if prevFail > 0 {
					st.RecoverStreak = 1
					st.RecoverSince = now
				} else {
					st.RecoverStreak++
					if st.RecoverSince == "" {
						st.RecoverSince = now
					}
				}
				st.FailStreak = 0
				st.FirstFailAt = ""
			}
			st.LastSeenAt = now
			if err := m.store.SaveEventState(st); err != nil {
				logger.Error("保存证书检测状态失败 key=%s 错误=%v", key, err)
				continue
			}
			if !hasIssue || st.FailStreak < certConfirmFailures {
				continue
			}
			out = append(out, candidate{
				Fingerprint: issue.Fingerprint,
				AlertType:   issue.AlertType,
				Target:      c.Domain,
				Level:       "warning",
				Title:       issue.Title,
				Message:     issue.Message,
				LastError:   issue.Message,
				Cooldown:    certCooldown,
				WarningKey:  issue.Fingerprint,
			})
		}
	}
	return out
}

type certIssue struct {
	Fingerprint string
	AlertType   string
	Title       string
	Message     string
}

func certIssues(c model.CertStatus) []certIssue {
	var out []certIssue
	domain := strings.TrimSpace(c.Domain)
	if domain == "" {
		return out
	}
	if !c.Present || !c.OK {
		msg := fmt.Sprintf("域名 %s 本地证书文件不存在或证书解析失败，请检查 cert.pem/fullchain.pem。", domain)
		out = append(out, certIssue{
			Fingerprint: "cert_expiry:" + domain + ":expired",
			AlertType:   "cert_expiry",
			Title:       fmt.Sprintf("FRPS SSL证书不可用 - %s", domain),
			Message:     msg,
		})
		return out
	}
	if issue, ok := certExpiryIssue(domain, "本地证书", c.DaysLeft, c.ExpiresAt); ok {
		out = append(out, issue)
	}
	if !c.TLSOK {
		msg := fmt.Sprintf("域名 %s 公网 TLS 检测失败，暂无法确认远端证书状态：%s。", domain, c.TLSError)
		out = append(out, certIssue{
			Fingerprint: "cert_tls_check_failed:" + domain,
			AlertType:   "cert_tls_check_failed",
			Title:       fmt.Sprintf("FRPS SSL证书在线检测失败 - %s", domain),
			Message:     msg,
		})
		return out
	}
	if c.TLSHasLocalCert && !c.TLSMatchLocal {
		msg := fmt.Sprintf("域名 %s 公网握手证书与本地证书不一致，请检查反向代理或证书部署。", domain)
		out = append(out, certIssue{
			Fingerprint: "cert_tls_check_failed:" + domain,
			AlertType:   "cert_tls_check_failed",
			Title:       fmt.Sprintf("FRPS SSL证书在线检测异常 - %s", domain),
			Message:     msg,
		})
	}
	if issue, ok := certExpiryIssue(domain, "公网握手证书", c.TLSDaysLeft, c.TLSExpiresAt); ok {
		out = append(out, issue)
	}
	return out
}

func certExpiryIssue(domain, source string, daysLeft *int, expiresAt string) (certIssue, bool) {
	if daysLeft == nil {
		return certIssue{}, false
	}
	stage, ok := certExpiryStage(*daysLeft)
	if !ok {
		return certIssue{}, false
	}
	titleStage := map[string]string{
		"30d":     "30天内到期",
		"7d":      "7天内到期",
		"1d":      "1天内到期",
		"expired": "已过期",
	}[stage]
	msg := fmt.Sprintf("域名 %s %s%s，剩余 %d 天，到期时间：%s。", domain, source, titleStage, *daysLeft, expiresAt)
	if stage == "expired" {
		msg = fmt.Sprintf("域名 %s %s已过期，剩余 %d 天，到期时间：%s。", domain, source, *daysLeft, expiresAt)
	}
	return certIssue{
		Fingerprint: "cert_expiry:" + domain + ":" + stage,
		AlertType:   "cert_expiry",
		Title:       fmt.Sprintf("FRPS SSL证书%s - %s", titleStage, domain),
		Message:     msg,
	}, true
}

func certExpiryStage(days int) (string, bool) {
	switch {
	case days <= 0:
		return "expired", true
	case days <= 1:
		return "1d", true
	case days <= 7:
		return "7d", true
	case days <= 30:
		return "30d", true
	default:
		return "", false
	}
}

func certKnownKeys(domain string) []string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil
	}
	return []string{
		"cert_expiry:" + domain + ":30d",
		"cert_expiry:" + domain + ":7d",
		"cert_expiry:" + domain + ":1d",
		"cert_expiry:" + domain + ":expired",
		"cert_tls_check_failed:" + domain,
	}
}

func (m *Manager) processGlobal(settings model.PublicSettings, alerts []candidate, total int) bool {
	now := time.Now()
	_ = m.store.DeleteAlertCandidatesOlderThan(now.Add(-globalCandidateWindow))
	if len(alerts) > 0 {
		if err := m.store.UpsertAlertCandidates(storeAlertCandidates(alerts), now); err != nil {
			logger.Error("写入全局告警候选缓存失败: %v", err)
		}
	}
	if total <= 0 || len(alerts) == 0 {
		return false
	}
	windowAlerts := alerts
	if cached, err := m.store.RecentAlertCandidates(now.Add(-globalCandidateWindow)); err != nil {
		logger.Error("读取全局告警候选缓存失败，退回当前轮判断: %v", err)
	} else {
		windowAlerts = alertCandidatesFromStore(cached)
	}
	observedTotal := total
	if observedTotal < len(windowAlerts) {
		observedTotal = len(windowAlerts)
	}
	global := len(windowAlerts) >= globalMinFailures || (observedTotal >= globalMinFailures && len(windowAlerts)*100 >= observedTotal*globalFailRatioPct)
	if !global {
		return false
	}
	var lines []string
	for _, c := range windowAlerts {
		lines = append(lines, "- "+c.Message)
		m.suppress(settings, c, "global_monitor_fault", "已触发全局故障，子告警只记录不通知")
	}
	msg := fmt.Sprintf("检测到疑似全局故障：%d/%d 个检测对象在 %d 分钟收敛窗口内异常。子告警已抑制。\n\n影响对象：\n%s", len(windowAlerts), observedTotal, int(globalCandidateWindow.Minutes()), strings.Join(lines, "\n"))
	m.fire(settings, candidate{
		Fingerprint: "global_monitor_fault",
		AlertType:   "global_fault",
		Target:      "monitor",
		Level:       "critical",
		Title:       fmt.Sprintf("FRPS 疑似全局故障 - %d/%d", len(windowAlerts), observedTotal),
		Message:     msg,
		LastError:   msg,
		Cooldown:    globalCooldown,
		WarningKey:  "global_monitor_fault",
	})
	return true
}

func storeAlertCandidates(items []candidate) []store.AlertCandidate {
	out := make([]store.AlertCandidate, 0, len(items))
	for _, item := range items {
		out = append(out, store.AlertCandidate{
			Fingerprint: item.Fingerprint,
			AlertType:   item.AlertType,
			Target:      item.Target,
			Level:       item.Level,
			Title:       item.Title,
			Message:     item.Message,
			LastError:   item.LastError,
			WarningKey:  item.WarningKey,
		})
	}
	return out
}

func alertCandidatesFromStore(items []store.AlertCandidate) []candidate {
	out := make([]candidate, 0, len(items))
	for _, item := range items {
		out = append(out, candidate{
			Fingerprint: item.Fingerprint,
			AlertType:   item.AlertType,
			Target:      item.Target,
			Level:       item.Level,
			Title:       item.Title,
			Message:     item.Message,
			LastError:   item.LastError,
			WarningKey:  item.WarningKey,
		})
	}
	return out
}

func (m *Manager) fire(settings model.PublicSettings, c candidate) {
	ev, err := m.store.UpsertAlertEvent(store.AlertEvent{
		Fingerprint:       c.Fingerprint,
		DefinitionID:      c.DefinitionID,
		AlertType:         c.AlertType,
		Target:            c.Target,
		Level:             c.Level,
		Status:            "firing",
		Title:             c.Title,
		Message:           c.Message,
		LastError:         c.LastError,
		ParentFingerprint: "",
		Suppressed:        false,
	})
	if err != nil {
		return
	}
	if c.WarningKey != "" {
		_ = m.store.SetWarning(c.WarningKey, c.Message)
	}
	if !smtpReady(settings) || !shouldNotify(ev.LastNotifyAt, c.Cooldown) {
		return
	}
	if err := mail.Send(settings, settings.SMTPAuthCode, c.Title, c.Message); err != nil {
		logger.Error("发送告警邮件失败 fingerprint=%s 错误=%v", c.Fingerprint, err)
		return
	}
	_ = m.store.MarkAlertEventNotified(c.Fingerprint, time.Now())
}

func (m *Manager) suppress(settings model.PublicSettings, c candidate, parent, reason string) {
	_, _ = m.store.UpsertAlertEvent(store.AlertEvent{
		Fingerprint:       c.Fingerprint,
		AlertType:         c.AlertType,
		Target:            c.Target,
		Level:             c.Level,
		Status:            "suppressed",
		Title:             c.Title,
		Message:           c.Message,
		LastError:         c.LastError,
		ParentFingerprint: parent,
		Suppressed:        true,
		SuppressReason:    reason,
	})
	if c.WarningKey != "" {
		_ = m.store.ClearWarning(c.WarningKey)
	}
}

func (m *Manager) resolve(settings model.PublicSettings, fingerprint, title, message string) {
	ev, changed, err := m.store.ResolveAlertEvent(fingerprint, title, message)
	if err != nil || !changed {
		return
	}
	if !smtpReady(settings) || ev.LastNotifyAt == "" {
		return
	}
	if err := mail.Send(settings, settings.SMTPAuthCode, title, message); err != nil {
		logger.Error("发送恢复通知邮件失败 fingerprint=%s 错误=%v", fingerprint, err)
		return
	}
	_ = m.store.MarkAlertEventNotified(fingerprint, time.Now())
}

func (m *Manager) clearRecoverProgress(key string) {
	st, err := m.store.GetEventState(key)
	if err != nil {
		return
	}
	st.RecoverStreak = 0
	st.RecoverSince = ""
	_ = m.store.SaveEventState(st)
}

// resolveAndClearRecoverProgress 将告警事件置为已恢复，并在成功更新事件后清零 event_alert_state 上的恢复确认进度（设计 11.2）。
func (m *Manager) resolveAndClearRecoverProgress(settings model.PublicSettings, fingerprint, title, message, recoverStateKey string) {
	ev, changed, err := m.store.ResolveAlertEvent(fingerprint, title, message)
	if err != nil || !changed {
		return
	}
	m.clearRecoverProgress(recoverStateKey)
	if !smtpReady(settings) || ev.LastNotifyAt == "" {
		return
	}
	if err := mail.Send(settings, settings.SMTPAuthCode, title, message); err != nil {
		logger.Error("发送恢复通知邮件失败 fingerprint=%s 错误=%v", fingerprint, err)
		return
	}
	_ = m.store.MarkAlertEventNotified(fingerprint, time.Now())
}

func (m *Manager) resolveMissingProxyAlerts(settings model.PublicSettings, proxies []model.ProxyTraffic) {
	for _, p := range proxies {
		if !p.Online || !p.Health.RecoveryConfirmed {
			continue
		}
		stateKey := "proxy_offline:" + p.Type + ":" + p.Name
		m.resolveAndClearRecoverProgress(settings, stateKey, fmt.Sprintf("FRPS 代理恢复通知 - %s", p.Name), fmt.Sprintf("代理 %s（类型：%s）已连续检测在线（恢复确认：连续成功≥3 次或稳定≥5 分钟）。", p.Name, p.Type), stateKey)
		_ = m.store.ClearWarning(stateKey)
		flapKey := "proxy_flapping:" + p.Type + ":" + p.Name
		m.resolveAndClearRecoverProgress(settings, flapKey, fmt.Sprintf("FRPS 代理抖动恢复 - %s", p.Name), fmt.Sprintf("代理 %s（类型：%s）状态已恢复稳定（恢复确认：连续成功≥3 次或稳定≥5 分钟）。", p.Name, p.Type), stateKey)
		_ = m.store.ClearWarning(flapKey)
	}
}

func (m *Manager) resolveMissingCertAlerts(settings model.PublicSettings, certs []model.CertStatus) {
	now := time.Now().UTC()
	for _, c := range certs {
		active := make(map[string]struct{})
		for _, issue := range certIssues(c) {
			active[issue.Fingerprint] = struct{}{}
		}
		for _, key := range certKnownKeys(c.Domain) {
			if _, ok := active[key]; ok {
				continue
			}
			st, err := m.store.GetEventState(key)
			if err != nil || !st.RecoveryConfirmed(now) {
				continue
			}
			m.resolveAndClearRecoverProgress(settings, key, fmt.Sprintf("FRPS SSL证书恢复通知 - %s", c.Domain), fmt.Sprintf("域名 %s 的证书检测已恢复正常（恢复确认：连续成功≥3 次或稳定≥5 分钟）。公网握手正常=%t，本地证书有效期=%s。", c.Domain, c.TLSOK, c.ExpiresAt), key)
			_ = m.store.ClearWarning(key)
		}
	}
}

func (m *Manager) suppressProxyChildren(proxies []model.ProxyTraffic, parent, reason string) {
	for _, p := range proxies {
		key := "proxy_offline:" + p.Type + ":" + p.Name
		_, _ = m.store.UpsertAlertEvent(store.AlertEvent{
			Fingerprint:       key,
			AlertType:         "proxy_offline",
			Target:            p.Name,
			Level:             "warning",
			Status:            "unknown",
			Title:             "代理状态未知",
			Message:           reason,
			ParentFingerprint: parent,
			Suppressed:        true,
			SuppressReason:    reason,
		})
		_ = m.store.ClearWarning(key)
	}
}

func (m *Manager) suppressCertChildren(certs []model.CertStatus, parent, reason string) {
	for _, c := range certs {
		key := "cert_tls_check_failed:" + c.Domain
		_, _ = m.store.UpsertAlertEvent(store.AlertEvent{
			Fingerprint:       key,
			AlertType:         "cert_tls_check_failed",
			Target:            c.Domain,
			Level:             "warning",
			Status:            "unknown",
			Title:             "证书检测状态未知",
			Message:           reason,
			ParentFingerprint: parent,
			Suppressed:        true,
			SuppressReason:    reason,
		})
		_ = m.store.ClearWarning(key)
	}
}

func smtpReady(settings model.PublicSettings) bool {
	return settings.SMTPEnabled && settings.SMTPAuthCode != "" && settings.SMTPHost != "" && settings.SMTPFrom != "" && settings.SMTPTo != ""
}

func shouldNotify(last string, cooldown time.Duration) bool {
	if last == "" {
		return true
	}
	t, err := time.Parse(time.RFC3339, last)
	if err != nil {
		return true
	}
	return time.Since(t) >= cooldown
}
