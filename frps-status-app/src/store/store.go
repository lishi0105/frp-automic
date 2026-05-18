package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/model"
	"github.com/google/uuid"
)

type User struct {
	ID                string
	Username          string
	PasswordHash      string
	PasswordSalt      string
	RecoveryEmail     string
	IsInitialPassword bool
}

func HashPassword(salt, password string) string {
	mac := hmac.New(sha256.New, []byte(salt))
	mac.Write([]byte(password))
	return hex.EncodeToString(mac.Sum(nil))
}

func GenerateSalt() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

type Store struct {
	db *sql.DB
}

type EventState struct {
	Key            string
	Active         bool
	FailStreak     int
	TotalChecks    int64
	OnlineChecks   int64
	SentAt         string
	LastChangeAt   string
	LastOfflineAt  string
	LastRecoveryAt string
	FirstFailAt    string
	LastSeenAt     string
	// 恢复确认（设计 11.2）：连续在线轮次、自本轮稳定在线起算时间（RFC3339）
	RecoverStreak int
	RecoverSince  string
}

// RecoveryConfirmed 为 true 表示满足「连续成功 ≥3 次」或「自 recover_since 起持续正常 ≥5 分钟」。
func (st EventState) RecoveryConfirmed(now time.Time) bool {
	const minStreak = 3
	const minStable = 5 * time.Minute
	if st.RecoverStreak >= minStreak {
		return true
	}
	s := strings.TrimSpace(st.RecoverSince)
	if s == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return false
	}
	return now.Sub(t) >= minStable
}

type AlertEvent struct {
	Fingerprint       string
	DefinitionID      string // 对应 alert_parent_definitions.id（UUID）
	AlertType         string
	Target            string
	Level             string
	Status            string
	Title             string
	Message           string
	FirstSeenAt       string
	LastSeenAt        string
	ResolvedAt        string
	OccurrenceCount   int64
	LastError         string
	LastNotifyAt      string
	ParentFingerprint string
	Suppressed        bool
	SuppressReason    string
	CreatedAt         string
	UpdatedAt         string
}

type AlertCandidate struct {
	Fingerprint string
	AlertType   string
	Target      string
	Level       string
	Title       string
	Message     string
	LastError   string
	WarningKey  string
	FirstSeenAt string
	LastSeenAt  string
}

// AlertParentDef 父级异常类定义：sort_rank 越小优先级越高；相同 sort_rank 为同一层级可同时触发。
type AlertParentDef struct {
	ID                string
	ParentKey         string
	SortRank          int
	BlocksDownstream  bool
	SuppressProxyCert bool
	Label             string
	Description       string
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) InitDB() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS app_meta (
			-- 元数据记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 元数据键名，例如 deploy_date。
			key TEXT NOT NULL UNIQUE,
			-- 元数据键值。
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_counters (
			-- 代理计数器记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- FRPS 代理名称。
			name TEXT NOT NULL,
			-- FRPS 代理类型，例如 tcp、http、https。
			type TEXT NOT NULL,
			-- 上一次采集到的入站累计字节数。
			last_in INTEGER NOT NULL,
			-- 上一次采集到的出站累计字节数。
			last_out INTEGER NOT NULL,
			-- 计数器最后更新时间，RFC3339 UTC 字符串。
			updated_at TEXT NOT NULL,
			-- 同一名称与类型的代理只保留一条计数器记录。
			UNIQUE(name,type)
		)`,
		`CREATE TABLE IF NOT EXISTS daily_traffic (
			-- 日代理流量记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 统计日期，格式 YYYY-MM-DD。
			day TEXT NOT NULL,
			-- FRPS 代理名称。
			proxy_name TEXT NOT NULL,
			-- FRPS 代理类型，例如 tcp、http、https。
			proxy_type TEXT NOT NULL,
			-- 当日新增入站字节数。
			in_bytes INTEGER NOT NULL DEFAULT 0,
			-- 当日新增出站字节数。
			out_bytes INTEGER NOT NULL DEFAULT 0,
			-- 当日观测到的最大连接数。
			peak_conns INTEGER NOT NULL DEFAULT 0,
			-- 每个日期、代理名称、代理类型只保留一条汇总记录。
			UNIQUE(day,proxy_name,proxy_type)
		)`,
		`CREATE TABLE IF NOT EXISTS event_alert_state (
			-- 告警状态记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 告警状态键，通常对应代理或检测对象。
			key TEXT NOT NULL UNIQUE,
			-- 最近一次发送告警通知的时间，RFC3339 UTC 字符串。
			sent_at TEXT NOT NULL,
			-- 当前异常是否处于激活状态，1 表示激活。
			active INTEGER NOT NULL DEFAULT 0,
			-- 连续失败次数。
			fail_streak INTEGER NOT NULL DEFAULT 0,
			-- 总检测次数。
			total_checks INTEGER NOT NULL DEFAULT 0,
			-- 在线或成功检测次数。
			online_checks INTEGER NOT NULL DEFAULT 0,
			-- 状态最近一次变化时间，RFC3339 UTC 字符串。
			last_change_at TEXT NOT NULL DEFAULT '',
			-- 最近一次离线时间，RFC3339 UTC 字符串。
			last_offline_at TEXT NOT NULL DEFAULT '',
			-- 最近一次恢复时间，RFC3339 UTC 字符串。
			last_recovery_at TEXT NOT NULL DEFAULT '',
			-- 本轮异常首次失败时间，RFC3339 UTC 字符串。
			first_fail_at TEXT NOT NULL DEFAULT '',
			-- 最近一次检测看到该对象的时间，RFC3339 UTC 字符串。
			last_seen_at TEXT NOT NULL DEFAULT '',
			-- 连续恢复成功次数。
			recover_streak INTEGER NOT NULL DEFAULT 0,
			-- 本轮稳定恢复起始时间，RFC3339 UTC 字符串。
			recover_since TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_status_events (
			-- 代理状态事件唯一 ID。
			id TEXT PRIMARY KEY,
			-- 代理状态键，通常对应代理名称与类型。
			key TEXT NOT NULL,
			-- 状态值，1 表示在线，0 表示离线。
			status INTEGER NOT NULL,
			-- 状态记录时间，RFC3339 UTC 字符串。
			at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS users (
			-- 用户唯一 ID。
			id TEXT PRIMARY KEY,
			-- 登录用户名。
			username TEXT NOT NULL UNIQUE,
			-- 密码哈希值。
			password_hash TEXT NOT NULL,
			-- 密码哈希盐值。
			password_salt TEXT NOT NULL,
			-- 找回或通知使用的邮箱地址。
			recovery_email TEXT NOT NULL DEFAULT '',
			-- 是否仍使用初始化密码，1 表示是。
			is_initial_password INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE TABLE IF NOT EXISTS warnings (
			-- 警告记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 警告键，用于去重与覆盖。
			key TEXT NOT NULL UNIQUE,
			-- 警告展示内容。
			message TEXT NOT NULL,
			-- 警告首次创建时间，RFC3339 UTC 字符串。
			created_at TEXT NOT NULL,
			-- 警告最近更新时间，RFC3339 UTC 字符串。
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS iface_counters (
			-- 网卡计数器记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 网卡名称。
			iface TEXT NOT NULL,
			-- 该网卡对应的公网 IP。
			public_ip TEXT NOT NULL,
			-- 上一次采集到的接收累计字节数。
			last_rx_bytes INTEGER NOT NULL,
			-- 上一次采集到的发送累计字节数。
			last_tx_bytes INTEGER NOT NULL,
			-- 计数器最后更新时间，RFC3339 UTC 字符串。
			updated_at TEXT NOT NULL,
			-- 同一网卡与公网 IP 只保留一条计数器记录。
			UNIQUE(iface,public_ip)
		)`,
		`CREATE TABLE IF NOT EXISTS daily_iface_traffic (
			-- 日网卡流量记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 统计日期，格式 YYYY-MM-DD。
			day TEXT NOT NULL,
			-- 网卡名称。
			iface TEXT NOT NULL,
			-- 该网卡对应的公网 IP。
			public_ip TEXT NOT NULL,
			-- 当日新增接收流量，单位 KB。
			rx_kb INTEGER NOT NULL DEFAULT 0,
			-- 当日新增发送流量，单位 KB。
			tx_kb INTEGER NOT NULL DEFAULT 0,
			-- 每个日期、网卡、公网 IP 只保留一条汇总记录。
			UNIQUE(day,iface,public_ip)
		)`,
		`CREATE TABLE IF NOT EXISTS alert_events (
			-- 告警事件记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 告警指纹，用于同类事件去重。
			fingerprint TEXT NOT NULL UNIQUE,
			-- 父级异常定义 ID，对应 alert_parent_definitions.id。
			definition_id TEXT NOT NULL DEFAULT '',
			-- 告警类型，例如 proxy_offline、cert_expiry。
			alert_type TEXT NOT NULL,
			-- 告警目标，例如代理名、域名或系统资源。
			target TEXT NOT NULL,
			-- 告警级别，例如 warning、critical。
			level TEXT NOT NULL,
			-- 告警状态，例如 active、resolved。
			status TEXT NOT NULL,
			-- 告警标题。
			title TEXT NOT NULL,
			-- 告警正文。
			message TEXT NOT NULL,
			-- 首次发现时间，RFC3339 UTC 字符串。
			first_seen_at TEXT NOT NULL,
			-- 最近发现时间，RFC3339 UTC 字符串。
			last_seen_at TEXT NOT NULL,
			-- 恢复时间，未恢复时为空。
			resolved_at TEXT NOT NULL DEFAULT '',
			-- 同一指纹累计出现次数。
			occurrence_count INTEGER NOT NULL DEFAULT 0,
			-- 最近一次错误详情。
			last_error TEXT NOT NULL DEFAULT '',
			-- 最近一次通知发送时间，未发送时为空。
			last_notify_at TEXT NOT NULL DEFAULT '',
			-- 抑制本事件的父级告警指纹。
			parent_fingerprint TEXT NOT NULL DEFAULT '',
			-- 是否被父级告警抑制，1 表示被抑制。
			suppressed INTEGER NOT NULL DEFAULT 0,
			-- 被抑制原因说明。
			suppress_reason TEXT NOT NULL DEFAULT '',
			-- 记录创建时间，RFC3339 UTC 字符串。
			created_at TEXT NOT NULL,
			-- 记录最近更新时间，RFC3339 UTC 字符串。
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS alert_candidate_cache (
			-- 候选告警缓存记录唯一 ID。
			id TEXT PRIMARY KEY,
			-- 候选告警指纹，用于同类候选事件去重。
			fingerprint TEXT NOT NULL UNIQUE,
			-- 候选告警类型。
			alert_type TEXT NOT NULL,
			-- 候选告警目标。
			target TEXT NOT NULL,
			-- 候选告警级别。
			level TEXT NOT NULL,
			-- 候选告警标题。
			title TEXT NOT NULL,
			-- 候选告警正文。
			message TEXT NOT NULL,
			-- 最近一次错误详情。
			last_error TEXT NOT NULL DEFAULT '',
			-- 关联 warning 记录的 key。
			warning_key TEXT NOT NULL DEFAULT '',
			-- 候选告警首次发现时间，RFC3339 UTC 字符串。
			first_seen_at TEXT NOT NULL,
			-- 候选告警最近发现时间，RFC3339 UTC 字符串。
			last_seen_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS alert_parent_definitions (
			-- 父级异常定义唯一 ID。
			id TEXT PRIMARY KEY,
			-- 父级异常键，作为业务识别名称。
			parent_key TEXT NOT NULL UNIQUE,
			-- 排序与层级优先级，数值越小越优先。
			sort_rank INTEGER NOT NULL,
			-- 是否阻断下游告警判断，1 表示阻断。
			blocks_downstream INTEGER NOT NULL DEFAULT 0,
			-- 是否抑制代理与证书类子告警，1 表示抑制。
			suppress_proxy_cert INTEGER NOT NULL DEFAULT 1,
			-- 父级异常展示名称。
			label TEXT NOT NULL DEFAULT '',
			-- 父级异常说明。
			description TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return logStoreErr("init database schema", err)
		}
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_proxy_status_events_key_at ON proxy_status_events(key, at)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_daily_iface_traffic_day ON daily_iface_traffic(day)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alert_events_status ON alert_events(status)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alert_candidate_cache_last_seen ON alert_candidate_cache(last_seen_at)`)

	const deployKey = "deploy_date"
	if _, err := s.db.Exec(`INSERT INTO app_meta(id,key,value) VALUES(?,?,?) ON CONFLICT(key) DO NOTHING`, uuid.NewString(), deployKey, time.Now().Format("2006-01-02")); err != nil {
		return logStoreErr("init deploy_date", err)
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_alert_parent_defs_rank ON alert_parent_definitions(sort_rank, parent_key)`)

	if err := seedAlertParentDefinitions(s.db); err != nil {
		return err
	}
	return nil
}

func seedAlertParentDefinitions(db *sql.DB) error {
	// sort_rank：0=独立告警（不抑制下游）；10=主机基础设施类（可多条同秩）；20=FRPS/反代服务入口；30=预留更下游父级。
	rows := []struct {
		key, label, desc   string
		rank               int
		blocks, suppressPC int
	}{
		{"traffic_threshold_in", "入站流量阈值", "独立告警：月入站流量达到提醒阈值。", 0, 0, 0},
		{"traffic_threshold_out", "出站流量阈值", "独立告警：月出站流量达到提醒阈值。", 0, 0, 0},
		{"traffic_threshold_total", "总流量阈值", "独立告警：月总流量达到提醒阈值。", 0, 0, 0},
		{"traffic_limit_in", "入站流量限额", "独立告警：月入站流量达到限额。", 0, 0, 0},
		{"traffic_limit_out", "出站流量限额", "独立告警：月出站流量达到限额。", 0, 0, 0},
		{"traffic_limit_total", "总流量限额", "独立告警：月总流量达到限额。", 0, 0, 0},
		{"monitor_network_down", "检测机网络/DNS/外网", "DNS 或 HTTPS 外网探测失败；抑制下游代理与证书检测结论。", 10, 1, 1},
		{"host_storage_pressure", "本机存储压力", "预留：磁盘危急、数据目录不可写等。", 10, 1, 1},
		{"host_memory_pressure", "内存压力", "预留：内存不足或 OOM 风险。", 10, 1, 1},
		{"host_cpu_saturation", "CPU 饱和", "预留：CPU 持续满载。", 10, 1, 1},
		{"frps_dashboard_unreachable", "FRPS 面板/反代", "无法拉取 FRPS 代理列表；抑制代理离线及证书检测类子告警。", 20, 0, 1},
	}
	for _, r := range rows {
		if _, err := db.Exec(`INSERT OR IGNORE INTO alert_parent_definitions(id,parent_key,sort_rank,blocks_downstream,suppress_proxy_cert,label,description) VALUES(?,?,?,?,?,?,?)`,
			uuid.NewString(), r.key, r.rank, r.blocks, r.suppressPC, r.label, r.desc); err != nil {
			return logStoreErr("seed alert_parent_definitions "+r.key, err)
		}
	}
	return nil
}

// ListAlertParentDefinitions 返回父级定义，按 sort_rank、parent_key 排序。
func (s *Store) ListAlertParentDefinitions() ([]AlertParentDef, error) {
	rows, err := s.db.Query(`SELECT id,parent_key,sort_rank,blocks_downstream,suppress_proxy_cert,label,description FROM alert_parent_definitions ORDER BY sort_rank ASC, parent_key ASC`)
	if err != nil {
		return nil, logStoreErr("list alert_parent_definitions", err)
	}
	defer rows.Close()
	var out []AlertParentDef
	for rows.Next() {
		var d AlertParentDef
		var blocks, supPC int
		if err := rows.Scan(&d.ID, &d.ParentKey, &d.SortRank, &blocks, &supPC, &d.Label, &d.Description); err != nil {
			return nil, logStoreErr("scan alert_parent_definitions", err)
		}
		d.BlocksDownstream = blocks == 1
		d.SuppressProxyCert = supPC == 1
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, logStoreErr("list alert_parent_definitions rows", err)
	}
	return out, nil
}

// AlertParentDefsByKey 父级 key -> 定义；表为空时返回内置默认，避免告警停摆。
func (s *Store) AlertParentDefsByKey() map[string]AlertParentDef {
	list, err := s.ListAlertParentDefinitions()
	if err != nil || len(list) == 0 {
		logger.Error("加载 alert_parent_definitions 失败或为空，使用内置父级排序: %v", err)
		return defaultAlertParentDefsByKey()
	}
	m := make(map[string]AlertParentDef, len(list))
	for _, d := range list {
		m[d.ParentKey] = d
	}
	return m
}

func defaultAlertParentDefsByKey() map[string]AlertParentDef {
	return map[string]AlertParentDef{
		"traffic_threshold_in": {
			ID: uuid.NewString(), ParentKey: "traffic_threshold_in", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "入站流量阈值", Description: "",
		},
		"traffic_threshold_out": {
			ID: uuid.NewString(), ParentKey: "traffic_threshold_out", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "出站流量阈值", Description: "",
		},
		"traffic_threshold_total": {
			ID: uuid.NewString(), ParentKey: "traffic_threshold_total", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "总流量阈值", Description: "",
		},
		"traffic_limit_in": {
			ID: uuid.NewString(), ParentKey: "traffic_limit_in", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "入站流量限额", Description: "",
		},
		"traffic_limit_out": {
			ID: uuid.NewString(), ParentKey: "traffic_limit_out", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "出站流量限额", Description: "",
		},
		"traffic_limit_total": {
			ID: uuid.NewString(), ParentKey: "traffic_limit_total", SortRank: 0,
			BlocksDownstream: false, SuppressProxyCert: false, Label: "总流量限额", Description: "",
		},
		"monitor_network_down": {
			ID: uuid.NewString(), ParentKey: "monitor_network_down", SortRank: 10,
			BlocksDownstream: true, SuppressProxyCert: true, Label: "检测机网络/DNS/外网", Description: "",
		},
		"host_storage_pressure": {
			ID: uuid.NewString(), ParentKey: "host_storage_pressure", SortRank: 10,
			BlocksDownstream: true, SuppressProxyCert: true, Label: "本机存储压力", Description: "",
		},
		"host_memory_pressure": {
			ID: uuid.NewString(), ParentKey: "host_memory_pressure", SortRank: 10,
			BlocksDownstream: true, SuppressProxyCert: true, Label: "内存压力", Description: "",
		},
		"host_cpu_saturation": {
			ID: uuid.NewString(), ParentKey: "host_cpu_saturation", SortRank: 10,
			BlocksDownstream: true, SuppressProxyCert: true, Label: "CPU 饱和", Description: "",
		},
		"frps_dashboard_unreachable": {
			ID: uuid.NewString(), ParentKey: "frps_dashboard_unreachable", SortRank: 20,
			BlocksDownstream: false, SuppressProxyCert: true, Label: "FRPS 面板/反代", Description: "",
		},
	}
}

// DeployDate 返回与数据库文件绑定的部署日期（YYYY-MM-DD），仅来自 app_meta。
func (s *Store) DeployDate() string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM app_meta WHERE key=?`, "deploy_date").Scan(&v)
	if err != nil || strings.TrimSpace(v) == "" {
		return ""
	}
	return strings.TrimSpace(v)
}

func (s *Store) RecordTraffic(proxies []model.ProxyTraffic) error {
	now := time.Now()
	day := now.Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return logStoreErr("begin record traffic transaction", err)
	}
	defer tx.Rollback()
	for _, p := range proxies {
		var lastIn, lastOut uint64
		err := tx.QueryRow(`SELECT last_in,last_out FROM proxy_counters WHERE name=? AND type=?`, p.Name, p.Type).Scan(&lastIn, &lastOut)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.Exec(`INSERT INTO proxy_counters(id,name,type,last_in,last_out,updated_at) VALUES(?,?,?,?,?,?)`, uuid.NewString(), p.Name, p.Type, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339))
			if err != nil {
				return logStoreErr("insert proxy counter "+p.Type+"/"+p.Name, err)
			}
			continue
		}
		if err != nil {
			return logStoreErr("query proxy counter "+p.Type+"/"+p.Name, err)
		}
		deltaIn, deltaOut := deltaCounter(lastIn, p.CurrentIn), deltaCounter(lastOut, p.CurrentOut)
		_, err = tx.Exec(`INSERT INTO daily_traffic(id,day,proxy_name,proxy_type,in_bytes,out_bytes,peak_conns) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(day,proxy_name,proxy_type) DO UPDATE SET in_bytes=in_bytes+excluded.in_bytes,out_bytes=out_bytes+excluded.out_bytes,peak_conns=MAX(daily_traffic.peak_conns,excluded.peak_conns)`, uuid.NewString(), day, p.Name, p.Type, deltaIn, deltaOut, p.CurConns)
		if err != nil {
			return logStoreErr("upsert daily traffic "+p.Type+"/"+p.Name, err)
		}
		_, err = tx.Exec(`UPDATE proxy_counters SET last_in=?, last_out=?, updated_at=? WHERE name=? AND type=?`, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339), p.Name, p.Type)
		if err != nil {
			return logStoreErr("update proxy counter "+p.Type+"/"+p.Name, err)
		}
	}
	return logStoreErr("commit record traffic transaction", tx.Commit())
}

func (s *Store) MonthTotals(month string) (uint64, uint64, error) {
	var in, out int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(in_bytes),0), COALESCE(SUM(out_bytes),0) FROM daily_traffic WHERE day LIKE ?`, month+"-%").Scan(&in, &out)
	return uint64(clampZero(in)), uint64(clampZero(out)), logStoreErr("query month totals "+month, err)
}

func (s *Store) MonthTotalForProxy(name, typ, month string) (uint64, uint64, error) {
	var in, out int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(in_bytes),0), COALESCE(SUM(out_bytes),0) FROM daily_traffic WHERE proxy_name=? AND proxy_type=? AND day LIKE ?`, name, typ, month+"-%").Scan(&in, &out)
	return uint64(clampZero(in)), uint64(clampZero(out)), logStoreErr("query month total for proxy "+typ+"/"+name, err)
}

func (s *Store) TotalForProxyBetween(name, typ, fromDay, toDay string) (uint64, uint64, error) {
	var in, out int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(in_bytes),0), COALESCE(SUM(out_bytes),0) FROM daily_traffic WHERE proxy_name=? AND proxy_type=? AND day >= ? AND day <= ?`, name, typ, fromDay, toDay).Scan(&in, &out)
	return uint64(clampZero(in)), uint64(clampZero(out)), logStoreErr("query proxy totals between "+fromDay+" and "+toDay+" "+typ+"/"+name, err)
}

func (s *Store) GetEventState(key string) (EventState, error) {
	var st EventState
	var active int
	err := s.db.QueryRow(`SELECT key,active,fail_streak,total_checks,online_checks,sent_at,last_change_at,last_offline_at,last_recovery_at,first_fail_at,last_seen_at,recover_streak,recover_since FROM event_alert_state WHERE key=?`, key).
		Scan(&st.Key, &active, &st.FailStreak, &st.TotalChecks, &st.OnlineChecks, &st.SentAt, &st.LastChangeAt, &st.LastOfflineAt, &st.LastRecoveryAt, &st.FirstFailAt, &st.LastSeenAt, &st.RecoverStreak, &st.RecoverSince)
	if errors.Is(err, sql.ErrNoRows) {
		return EventState{Key: key}, nil
	}
	if err != nil {
		return st, logStoreErr("get event state "+key, err)
	}
	st.Active = active == 1
	return st, nil
}

func (s *Store) SaveEventState(st EventState) error {
	active := 0
	if st.Active {
		active = 1
	}
	_, err := s.db.Exec(`INSERT INTO event_alert_state(id,key,sent_at,active,fail_streak,total_checks,online_checks,last_change_at,last_offline_at,last_recovery_at,first_fail_at,last_seen_at,recover_streak,recover_since)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(key) DO UPDATE SET sent_at=excluded.sent_at,active=excluded.active,fail_streak=excluded.fail_streak,total_checks=excluded.total_checks,online_checks=excluded.online_checks,last_change_at=excluded.last_change_at,last_offline_at=excluded.last_offline_at,last_recovery_at=excluded.last_recovery_at,first_fail_at=excluded.first_fail_at,last_seen_at=excluded.last_seen_at,recover_streak=excluded.recover_streak,recover_since=excluded.recover_since`,
		uuid.NewString(), st.Key, st.SentAt, active, st.FailStreak, st.TotalChecks, st.OnlineChecks, st.LastChangeAt, st.LastOfflineAt, st.LastRecoveryAt, st.FirstFailAt, st.LastSeenAt, st.RecoverStreak, st.RecoverSince)
	return logStoreErr("save event state "+st.Key, err)
}

func (s *Store) AddProxyStatusEvent(key string, online bool, at time.Time) error {
	status := 0
	if online {
		status = 1
	}
	_, err := s.db.Exec(`INSERT INTO proxy_status_events(id,key,status,at) VALUES(?,?,?,?)`, uuid.NewString(), key, status, at.UTC().Format(time.RFC3339))
	return logStoreErr("add proxy status event "+key, err)
}

func (s *Store) ProxyFlapCountSince(key string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxy_status_events WHERE key=? AND at>=?`, key, since.UTC().Format(time.RFC3339)).Scan(&count)
	return count, logStoreErr("proxy flap count "+key, err)
}

func (s *Store) GetAlertEvent(fingerprint string) (AlertEvent, error) {
	var ev AlertEvent
	var suppressed int
	err := s.db.QueryRow(`SELECT fingerprint,definition_id,alert_type,target,level,status,title,message,first_seen_at,last_seen_at,resolved_at,occurrence_count,last_error,last_notify_at,parent_fingerprint,suppressed,suppress_reason,created_at,updated_at FROM alert_events WHERE fingerprint=?`, fingerprint).
		Scan(&ev.Fingerprint, &ev.DefinitionID, &ev.AlertType, &ev.Target, &ev.Level, &ev.Status, &ev.Title, &ev.Message, &ev.FirstSeenAt, &ev.LastSeenAt, &ev.ResolvedAt, &ev.OccurrenceCount, &ev.LastError, &ev.LastNotifyAt, &ev.ParentFingerprint, &suppressed, &ev.SuppressReason, &ev.CreatedAt, &ev.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertEvent{Fingerprint: fingerprint}, nil
	}
	if err != nil {
		return ev, logStoreErr("get alert event "+fingerprint, err)
	}
	ev.Suppressed = suppressed == 1
	return ev, nil
}

func (s *Store) UpsertAlertEvent(ev AlertEvent) (AlertEvent, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	existing, err := s.GetAlertEvent(ev.Fingerprint)
	if err != nil {
		return ev, err
	}
	if existing.CreatedAt == "" {
		if ev.FirstSeenAt == "" {
			ev.FirstSeenAt = now
		}
		if ev.LastSeenAt == "" {
			ev.LastSeenAt = now
		}
		if ev.CreatedAt == "" {
			ev.CreatedAt = now
		}
		ev.UpdatedAt = now
		if ev.OccurrenceCount <= 0 {
			ev.OccurrenceCount = 1
		}
	} else {
		ev.FirstSeenAt = existing.FirstSeenAt
		ev.CreatedAt = existing.CreatedAt
		ev.LastNotifyAt = existing.LastNotifyAt
		if ev.DefinitionID == "" {
			ev.DefinitionID = existing.DefinitionID
		}
		if ev.LastSeenAt == "" {
			ev.LastSeenAt = now
		}
		ev.UpdatedAt = now
		ev.OccurrenceCount = existing.OccurrenceCount + 1
	}
	suppressed := 0
	if ev.Suppressed {
		suppressed = 1
	}
	_, err = s.db.Exec(`INSERT INTO alert_events(id,fingerprint,definition_id,alert_type,target,level,status,title,message,first_seen_at,last_seen_at,resolved_at,occurrence_count,last_error,last_notify_at,parent_fingerprint,suppressed,suppress_reason,created_at,updated_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(fingerprint) DO UPDATE SET definition_id=excluded.definition_id,alert_type=excluded.alert_type,target=excluded.target,level=excluded.level,status=excluded.status,title=excluded.title,message=excluded.message,last_seen_at=excluded.last_seen_at,resolved_at=excluded.resolved_at,occurrence_count=excluded.occurrence_count,last_error=excluded.last_error,parent_fingerprint=excluded.parent_fingerprint,suppressed=excluded.suppressed,suppress_reason=excluded.suppress_reason,updated_at=excluded.updated_at`,
		uuid.NewString(), ev.Fingerprint, ev.DefinitionID, ev.AlertType, ev.Target, ev.Level, ev.Status, ev.Title, ev.Message, ev.FirstSeenAt, ev.LastSeenAt, ev.ResolvedAt, ev.OccurrenceCount, ev.LastError, ev.LastNotifyAt, ev.ParentFingerprint, suppressed, ev.SuppressReason, ev.CreatedAt, ev.UpdatedAt)
	return ev, logStoreErr("upsert alert event "+ev.Fingerprint, err)
}

func (s *Store) MarkAlertEventNotified(fingerprint string, at time.Time) error {
	_, err := s.db.Exec(`UPDATE alert_events SET last_notify_at=?, updated_at=? WHERE fingerprint=?`, at.UTC().Format(time.RFC3339), at.UTC().Format(time.RFC3339), fingerprint)
	return logStoreErr("mark alert event notified "+fingerprint, err)
}

func (s *Store) ResolveAlertEvent(fingerprint, title, message string) (AlertEvent, bool, error) {
	ev, err := s.GetAlertEvent(fingerprint)
	if err != nil || ev.CreatedAt == "" || ev.Status == "resolved" {
		return ev, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	ev.Status = "resolved"
	ev.Title = title
	ev.Message = message
	ev.ResolvedAt = now
	ev.LastSeenAt = now
	ev.UpdatedAt = now
	ev.Suppressed = false
	ev.SuppressReason = ""
	suppressed := 0
	if ev.Suppressed {
		suppressed = 1
	}
	_, err = s.db.Exec(`UPDATE alert_events SET status=?,title=?,message=?,resolved_at=?,last_seen_at=?,suppressed=?,suppress_reason=?,updated_at=? WHERE fingerprint=?`, ev.Status, ev.Title, ev.Message, ev.ResolvedAt, ev.LastSeenAt, suppressed, ev.SuppressReason, ev.UpdatedAt, ev.Fingerprint)
	return ev, true, logStoreErr("resolve alert event "+fingerprint, err)
}

func (s *Store) UpsertAlertCandidates(items []AlertCandidate, at time.Time) error {
	if len(items) == 0 {
		return nil
	}
	now := at.UTC().Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return logStoreErr("begin upsert alert candidates transaction", err)
	}
	defer tx.Rollback()
	for _, item := range items {
		if strings.TrimSpace(item.Fingerprint) == "" {
			continue
		}
		_, err := tx.Exec(`INSERT INTO alert_candidate_cache(id,fingerprint,alert_type,target,level,title,message,last_error,warning_key,first_seen_at,last_seen_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(fingerprint) DO UPDATE SET alert_type=excluded.alert_type,target=excluded.target,level=excluded.level,title=excluded.title,message=excluded.message,last_error=excluded.last_error,warning_key=excluded.warning_key,last_seen_at=excluded.last_seen_at`,
			uuid.NewString(), item.Fingerprint, item.AlertType, item.Target, item.Level, item.Title, item.Message, item.LastError, item.WarningKey, now, now)
		if err != nil {
			return logStoreErr("upsert alert candidate "+item.Fingerprint, err)
		}
	}
	return logStoreErr("commit upsert alert candidates transaction", tx.Commit())
}

func (s *Store) RecentAlertCandidates(since time.Time) ([]AlertCandidate, error) {
	rows, err := s.db.Query(`SELECT fingerprint,alert_type,target,level,title,message,last_error,warning_key,first_seen_at,last_seen_at FROM alert_candidate_cache WHERE last_seen_at>=? ORDER BY first_seen_at ASC, fingerprint ASC`, since.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, logStoreErr("query recent alert candidates", err)
	}
	defer rows.Close()
	var out []AlertCandidate
	for rows.Next() {
		var item AlertCandidate
		if err := rows.Scan(&item.Fingerprint, &item.AlertType, &item.Target, &item.Level, &item.Title, &item.Message, &item.LastError, &item.WarningKey, &item.FirstSeenAt, &item.LastSeenAt); err != nil {
			return nil, logStoreErr("scan alert candidate", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, logStoreErr("recent alert candidates rows", err)
	}
	return out, nil
}

func (s *Store) DeleteAlertCandidatesOlderThan(cutoff time.Time) error {
	_, err := s.db.Exec(`DELETE FROM alert_candidate_cache WHERE last_seen_at<?`, cutoff.UTC().Format(time.RFC3339))
	return logStoreErr("delete stale alert candidates", err)
}

func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return logStoreErr("vacuum database", err)
}

func (s *Store) Purge(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return 0, logStoreErr("begin purge transaction", err)
	}
	defer tx.Rollback()

	res1, err := tx.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoff)
	if err != nil {
		return 0, logStoreErr("purge daily traffic", err)
	}
	res2, err := tx.Exec(`DELETE FROM daily_iface_traffic WHERE day < ?`, cutoff)
	if err != nil {
		return 0, logStoreErr("purge daily iface traffic", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, logStoreErr("commit purge transaction", err)
	}
	n1, _ := res1.RowsAffected()
	n2, _ := res2.RowsAffected()
	return n1 + n2, nil
}

func (s *Store) DailyTraffic() ([]map[string]any, error) {
	rows, err := s.db.Query(`SELECT day,proxy_name,proxy_type,in_bytes,out_bytes,peak_conns FROM daily_traffic ORDER BY day,proxy_name,proxy_type`)
	if err != nil {
		return nil, logStoreErr("query daily traffic", err)
	}
	defer rows.Close()
	var data []map[string]any
	for rows.Next() {
		var day, name, typ string
		var in, out, peak int64
		if err := rows.Scan(&day, &name, &typ, &in, &out, &peak); err != nil {
			return nil, logStoreErr("scan daily traffic row", err)
		}
		data = append(data, map[string]any{"day": day, "name": name, "type": typ, "in": clampZero(in), "out": clampZero(out), "peak_conns": clampZero(peak)})
	}
	return data, nil
}

func (s *Store) RecordInterfaceTraffic(iface, publicIP string, rxBytes, txBytes uint64, now time.Time) error {
	day := now.Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return logStoreErr("begin record iface traffic transaction", err)
	}
	defer tx.Rollback()

	var lastRx, lastTx uint64
	err = tx.QueryRow(`SELECT last_rx_bytes,last_tx_bytes FROM iface_counters WHERE iface=? AND public_ip=?`, iface, publicIP).Scan(&lastRx, &lastTx)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO iface_counters(id,iface,public_ip,last_rx_bytes,last_tx_bytes,updated_at) VALUES(?,?,?,?,?,?)`,
			uuid.NewString(), iface, publicIP, rxBytes, txBytes, now.UTC().Format(time.RFC3339))
		if err != nil {
			return logStoreErr("insert iface counter "+iface+"/"+publicIP, err)
		}
		return logStoreErr("commit record iface traffic init transaction", tx.Commit())
	}
	if err != nil {
		return logStoreErr("query iface counter "+iface+"/"+publicIP, err)
	}

	deltaRx := deltaCounter(lastRx, rxBytes)
	deltaTx := deltaCounter(lastTx, txBytes)
	rxKB := deltaRx / 1024
	txKB := deltaTx / 1024

	if rxKB > 0 || txKB > 0 {
		_, err = tx.Exec(`INSERT INTO daily_iface_traffic(id,day,iface,public_ip,rx_kb,tx_kb) VALUES(?,?,?,?,?,?)
ON CONFLICT(day,iface,public_ip) DO UPDATE SET rx_kb=rx_kb+excluded.rx_kb,tx_kb=tx_kb+excluded.tx_kb`,
			uuid.NewString(), day, iface, publicIP, rxKB, txKB)
		if err != nil {
			return logStoreErr("upsert daily iface traffic "+iface+"/"+publicIP, err)
		}
	}
	_, err = tx.Exec(`UPDATE iface_counters SET last_rx_bytes=?,last_tx_bytes=?,updated_at=? WHERE iface=? AND public_ip=?`,
		rxBytes, txBytes, now.UTC().Format(time.RFC3339), iface, publicIP)
	if err != nil {
		return logStoreErr("update iface counter "+iface+"/"+publicIP, err)
	}
	return logStoreErr("commit record iface traffic transaction", tx.Commit())
}

// PurgeTrafficDayBefore 删除 day 早于 cutoffDay 的流量日表与网卡日表行（cutoff 当日及之后保留）。
func (s *Store) PurgeTrafficDayBefore(cutoffDay string) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, logStoreErr("begin purge traffic before day transaction", err)
	}
	defer tx.Rollback()
	res1, err := tx.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoffDay)
	if err != nil {
		return 0, logStoreErr("purge daily traffic before day", err)
	}
	res2, err := tx.Exec(`DELETE FROM daily_iface_traffic WHERE day < ?`, cutoffDay)
	if err != nil {
		return 0, logStoreErr("purge daily iface traffic before day", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, logStoreErr("commit purge traffic before day", err)
	}
	n1, _ := res1.RowsAffected()
	n2, _ := res2.RowsAffected()
	return n1 + n2, nil
}

// TrafficDistinctDayCount 返回日流量与网卡日流量并集中不同日期的个数。
func (s *Store) TrafficDistinctDayCount() (int, error) {
	var n int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT DISTINCT day FROM daily_traffic
			UNION
			SELECT DISTINCT day FROM daily_iface_traffic
		)`).Scan(&n)
	return n, logStoreErr("count distinct traffic days", err)
}

// OldestTrafficDay 返回所有日流量相关表中的最早日期；若无数据则返回空字符串。
func (s *Store) OldestTrafficDay() (string, error) {
	var min sql.NullString
	err := s.db.QueryRow(`
		SELECT MIN(day) FROM (
			SELECT day FROM daily_traffic
			UNION ALL
			SELECT day FROM daily_iface_traffic
		)`).Scan(&min)
	if err != nil {
		return "", logStoreErr("query oldest traffic day", err)
	}
	if !min.Valid {
		return "", nil
	}
	return min.String, nil
}

func (s *Store) DailyInterfaceTraffic(fromDay, toDay string) ([]map[string]any, error) {
	query := `SELECT day,iface,public_ip,rx_kb,tx_kb FROM daily_iface_traffic`
	args := []any{}
	where := []string{}
	if strings.TrimSpace(fromDay) != "" {
		where = append(where, "day >= ?")
		args = append(args, fromDay)
	}
	if strings.TrimSpace(toDay) != "" {
		where = append(where, "day <= ?")
		args = append(args, toDay)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY day,iface,public_ip"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, logStoreErr("query daily iface traffic", err)
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var day, iface, publicIP string
		var rxKB, txKB int64
		if err := rows.Scan(&day, &iface, &publicIP, &rxKB, &txKB); err != nil {
			return nil, logStoreErr("scan daily iface traffic row", err)
		}
		out = append(out, map[string]any{
			"day":       day,
			"iface":     iface,
			"public_ip": publicIP,
			"rx_kb":     clampZero(rxKB),
			"tx_kb":     clampZero(txKB),
		})
	}
	return out, nil
}

func (s *Store) MonthInterfaceTotals(month string) (uint64, uint64, error) {
	var rxKB, txKB int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(rx_kb),0), COALESCE(SUM(tx_kb),0) FROM daily_iface_traffic WHERE day LIKE ?`, month+"-%").Scan(&rxKB, &txKB)
	return uint64(clampZero(rxKB)) * 1024, uint64(clampZero(txKB)) * 1024, logStoreErr("query month iface totals "+month, err)
}

func (s *Store) InterfaceTotalsBetween(fromDay, toDay string) (uint64, uint64, error) {
	var rxKB, txKB int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(rx_kb),0), COALESCE(SUM(tx_kb),0) FROM daily_iface_traffic WHERE day >= ? AND day <= ?`, fromDay, toDay).Scan(&rxKB, &txKB)
	return uint64(clampZero(rxKB)) * 1024, uint64(clampZero(txKB)) * 1024, logStoreErr("query iface totals between "+fromDay+" and "+toDay, err)
}

func deltaCounter(old, current uint64) uint64 {
	if current >= old {
		return current - old
	}
	return current
}

func clampZero(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

type Warning struct {
	Key       string `json:"key"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) SetWarning(key, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO warnings(id,key,message,created_at,updated_at) VALUES(?,?,?,?,?)
         ON CONFLICT(key) DO UPDATE SET message=excluded.message, updated_at=excluded.updated_at`,
		uuid.NewString(), key, message, now, now,
	)
	return logStoreErr("set warning "+key, err)
}

func (s *Store) ClearWarning(key string) error {
	_, err := s.db.Exec(`DELETE FROM warnings WHERE key=?`, key)
	return logStoreErr("clear warning "+key, err)
}

func (s *Store) GetWarnings() ([]Warning, error) {
	rows, err := s.db.Query(`SELECT key,message,created_at FROM warnings ORDER BY created_at`)
	if err != nil {
		return nil, logStoreErr("query warnings", err)
	}
	defer rows.Close()
	var out []Warning
	for rows.Next() {
		var w Warning
		if err := rows.Scan(&w.Key, &w.Message, &w.CreatedAt); err != nil {
			return nil, logStoreErr("scan warning row", err)
		}
		out = append(out, w)
	}
	if out == nil {
		out = []Warning{}
	}
	return out, nil
}

// SeedUser creates the single admin user if none exists yet.
// Called on startup using credentials from environment variables.
func (s *Store) SeedUser(username, password string) error {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if count > 0 {
		return nil
	}
	salt, err := GenerateSalt()
	if err != nil {
		return logStoreErr("generate initial user salt", err)
	}
	_, err = s.db.Exec(
		`INSERT INTO users(id,username,password_hash,password_salt,recovery_email,is_initial_password) VALUES(?,?,?,?,'',1)`,
		uuid.NewString(), username, HashPassword(salt, password), salt,
	)
	return logStoreErr("seed initial user", err)
}

func (s *Store) GetUser() (User, error) {
	var u User
	var isInitial int
	err := s.db.QueryRow(`SELECT id,username,password_hash,password_salt,recovery_email,is_initial_password FROM users LIMIT 1`).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.PasswordSalt, &u.RecoveryEmail, &isInitial)
	u.IsInitialPassword = isInitial == 1
	return u, logStoreErr("get user", err)
}

func (s *Store) UpdateUserCredentials(id, username, passwordHash, passwordSalt string) error {
	_, err := s.db.Exec(
		`UPDATE users SET username=?,password_hash=?,password_salt=?,is_initial_password=0 WHERE id=?`,
		username, passwordHash, passwordSalt, id,
	)
	return logStoreErr("update user credentials "+username, err)
}

func (s *Store) UpdateUserRecoveryEmail(id, email string) error {
	_, err := s.db.Exec(`UPDATE users SET recovery_email=? WHERE id=?`, email, id)
	return logStoreErr("update user recovery email", err)
}

func logStoreErr(op string, err error) error {
	if err != nil {
		logger.Error("数据库操作失败 操作=%s 错误=%v", op, err)
	}
	return err
}
