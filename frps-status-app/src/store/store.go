package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"math"
	"strconv"
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
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) InitDB() error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS proxy_counters (name TEXT NOT NULL, type TEXT NOT NULL, last_in INTEGER NOT NULL, last_out INTEGER NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY(name,type))`,
		`CREATE TABLE IF NOT EXISTS daily_traffic (day TEXT NOT NULL, proxy_name TEXT NOT NULL, proxy_type TEXT NOT NULL, in_bytes INTEGER NOT NULL DEFAULT 0, out_bytes INTEGER NOT NULL DEFAULT 0, peak_conns INTEGER NOT NULL DEFAULT 0, PRIMARY KEY(day,proxy_name,proxy_type))`,
		`CREATE TABLE IF NOT EXISTS alert_state (month TEXT NOT NULL, direction TEXT NOT NULL, sent_at TEXT NOT NULL, PRIMARY KEY(month,direction))`,
		`CREATE TABLE IF NOT EXISTS event_alert_state (key TEXT PRIMARY KEY, sent_at TEXT NOT NULL, active INTEGER NOT NULL DEFAULT 0, fail_streak INTEGER NOT NULL DEFAULT 0, total_checks INTEGER NOT NULL DEFAULT 0, online_checks INTEGER NOT NULL DEFAULT 0, last_change_at TEXT NOT NULL DEFAULT '', last_offline_at TEXT NOT NULL DEFAULT '', last_recovery_at TEXT NOT NULL DEFAULT '', first_fail_at TEXT NOT NULL DEFAULT '', last_seen_at TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS proxy_status_events (key TEXT NOT NULL, status INTEGER NOT NULL, at TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE, password_hash TEXT NOT NULL, password_salt TEXT NOT NULL, recovery_email TEXT NOT NULL DEFAULT '', is_initial_password INTEGER NOT NULL DEFAULT 1)`,
		`CREATE TABLE IF NOT EXISTS warnings (key TEXT PRIMARY KEY, message TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return logStoreErr("init database schema", err)
		}
	}
	_, _ = s.db.Exec(`ALTER TABLE daily_traffic ADD COLUMN peak_conns INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE users ADD COLUMN is_initial_password INTEGER NOT NULL DEFAULT 1`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN active INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN fail_streak INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN total_checks INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN online_checks INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN last_change_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN last_offline_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN last_recovery_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN first_fail_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`ALTER TABLE event_alert_state ADD COLUMN last_seen_at TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_proxy_status_events_key_at ON proxy_status_events(key, at)`)
	defaults := map[string]string{"alert_in_gb": "0", "alert_out_gb": "0", "alert_total_gb": "0", "smtp_port": "465", "smtp_enabled": "false", "alert_proxy_offline": "false", "alert_cert_expiry": "false", "alert_cert_days": "15", "smtp_verified": "false"}
	for k, v := range defaults {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, k, v); err != nil {
			return logStoreErr("init default setting "+k, err)
		}
	}
	return nil
}

func (s *Store) Setting(key string) string {
	var v string
	_ = s.db.QueryRow(`SELECT value FROM settings WHERE key=?`, key).Scan(&v)
	return v
}

func (s *Store) SaveSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return logStoreErr("save setting "+key, err)
}

func (s *Store) PublicSettings() (model.PublicSettings, error) {
	return model.PublicSettings{
		AlertInGB:         parseFloat(s.Setting("alert_in_gb")),
		AlertOutGB:        parseFloat(s.Setting("alert_out_gb")),
		AlertTotalGB:      parseFloat(s.Setting("alert_total_gb")),
		SMTPHost:          s.Setting("smtp_host"),
		SMTPPort:          int(parseFloatDefault(s.Setting("smtp_port"), 465)),
		SMTPUser:          s.Setting("smtp_user"),
		SMTPFrom:          s.Setting("smtp_from"),
		SMTPTo:            s.Setting("smtp_to"),
		SMTPEnabled:       strings.EqualFold(s.Setting("smtp_enabled"), "true"),
		SMTPAuthCode:      s.Setting("smtp_auth_code"),
		AlertProxyOffline: strings.EqualFold(s.Setting("alert_proxy_offline"), "true"),
		AlertCertExpiry:   strings.EqualFold(s.Setting("alert_cert_expiry"), "true"),
		AlertCertDays:     int(parseFloatDefault(s.Setting("alert_cert_days"), 15)),
	}, nil
}

func (s *Store) RecordTraffic(proxies []model.ProxyTraffic) error {
	now := time.Now()
	day := now.Format("2006-01-02")
	cutoff := now.AddDate(0, 0, -30).Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return logStoreErr("begin record traffic transaction", err)
	}
	defer tx.Rollback()
	for _, p := range proxies {
		var lastIn, lastOut uint64
		err := tx.QueryRow(`SELECT last_in,last_out FROM proxy_counters WHERE name=? AND type=?`, p.Name, p.Type).Scan(&lastIn, &lastOut)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.Exec(`INSERT INTO proxy_counters(name,type,last_in,last_out,updated_at) VALUES(?,?,?,?,?)`, p.Name, p.Type, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339))
			if err != nil {
				return logStoreErr("insert proxy counter "+p.Type+"/"+p.Name, err)
			}
			continue
		}
		if err != nil {
			return logStoreErr("query proxy counter "+p.Type+"/"+p.Name, err)
		}
		deltaIn, deltaOut := deltaCounter(lastIn, p.CurrentIn), deltaCounter(lastOut, p.CurrentOut)
		_, err = tx.Exec(`INSERT INTO daily_traffic(day,proxy_name,proxy_type,in_bytes,out_bytes,peak_conns) VALUES(?,?,?,?,?,?)
ON CONFLICT(day,proxy_name,proxy_type) DO UPDATE SET in_bytes=in_bytes+excluded.in_bytes,out_bytes=out_bytes+excluded.out_bytes,peak_conns=MAX(daily_traffic.peak_conns,excluded.peak_conns)`, day, p.Name, p.Type, deltaIn, deltaOut, p.CurConns)
		if err != nil {
			return logStoreErr("upsert daily traffic "+p.Type+"/"+p.Name, err)
		}
		_, err = tx.Exec(`UPDATE proxy_counters SET last_in=?, last_out=?, updated_at=? WHERE name=? AND type=?`, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339), p.Name, p.Type)
		if err != nil {
			return logStoreErr("update proxy counter "+p.Type+"/"+p.Name, err)
		}
	}
	if _, err := tx.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoff); err != nil {
		return logStoreErr("delete expired daily traffic", err)
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

func (s *Store) AlertSent(month, direction string) bool {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM alert_state WHERE month=? AND direction=?`, month, direction).Scan(&count)
	return count > 0
}

func (s *Store) MarkAlertSent(month, direction string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO alert_state(month,direction,sent_at) VALUES(?,?,?)`, month, direction, time.Now().UTC().Format(time.RFC3339))
	return logStoreErr("mark alert sent "+month+"/"+direction, err)
}

func (s *Store) EventAlertSent(key string) bool {
	var active int
	err := s.db.QueryRow(`SELECT active FROM event_alert_state WHERE key=?`, key).Scan(&active)
	return err == nil && active == 1
}

func (s *Store) SetEventAlert(key string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`INSERT INTO event_alert_state(key,sent_at,active,last_seen_at) VALUES(?,?,1,?)
ON CONFLICT(key) DO UPDATE SET sent_at=excluded.sent_at,active=1,last_seen_at=excluded.last_seen_at`, key, now, now)
	return logStoreErr("set event alert "+key, err)
}

func (s *Store) ClearEventAlert(key string) error {
	_, err := s.db.Exec(`UPDATE event_alert_state SET active=0, fail_streak=0, first_fail_at='', last_recovery_at=? WHERE key=?`, time.Now().UTC().Format(time.RFC3339), key)
	return logStoreErr("clear event alert "+key, err)
}

func (s *Store) GetEventState(key string) (EventState, error) {
	var st EventState
	var active int
	err := s.db.QueryRow(`SELECT key,active,fail_streak,total_checks,online_checks,sent_at,last_change_at,last_offline_at,last_recovery_at,first_fail_at,last_seen_at FROM event_alert_state WHERE key=?`, key).
		Scan(&st.Key, &active, &st.FailStreak, &st.TotalChecks, &st.OnlineChecks, &st.SentAt, &st.LastChangeAt, &st.LastOfflineAt, &st.LastRecoveryAt, &st.FirstFailAt, &st.LastSeenAt)
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
	_, err := s.db.Exec(`INSERT INTO event_alert_state(key,sent_at,active,fail_streak,total_checks,online_checks,last_change_at,last_offline_at,last_recovery_at,first_fail_at,last_seen_at)
VALUES(?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(key) DO UPDATE SET sent_at=excluded.sent_at,active=excluded.active,fail_streak=excluded.fail_streak,total_checks=excluded.total_checks,online_checks=excluded.online_checks,last_change_at=excluded.last_change_at,last_offline_at=excluded.last_offline_at,last_recovery_at=excluded.last_recovery_at,first_fail_at=excluded.first_fail_at,last_seen_at=excluded.last_seen_at`,
		st.Key, st.SentAt, active, st.FailStreak, st.TotalChecks, st.OnlineChecks, st.LastChangeAt, st.LastOfflineAt, st.LastRecoveryAt, st.FirstFailAt, st.LastSeenAt)
	return logStoreErr("save event state "+st.Key, err)
}

func (s *Store) AddProxyStatusEvent(key string, online bool, at time.Time) error {
	status := 0
	if online {
		status = 1
	}
	_, err := s.db.Exec(`INSERT INTO proxy_status_events(key,status,at) VALUES(?,?,?)`, key, status, at.UTC().Format(time.RFC3339))
	return logStoreErr("add proxy status event "+key, err)
}

func (s *Store) ProxyFlapCountSince(key string, since time.Time) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM proxy_status_events WHERE key=? AND at>=?`, key, since.UTC().Format(time.RFC3339)).Scan(&count)
	return count, logStoreErr("proxy flap count "+key, err)
}

func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return logStoreErr("vacuum database", err)
}

func (s *Store) Purge(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	res, err := s.db.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoff)
	if err != nil {
		return 0, logStoreErr("purge daily traffic", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
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

func parseFloat(v string) float64 {
	f, _ := strconv.ParseFloat(v, 64)
	if math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
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

type Warning struct {
	Key       string `json:"key"`
	Message   string `json:"message"`
	CreatedAt string `json:"created_at"`
}

func (s *Store) SetWarning(key, message string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(
		`INSERT INTO warnings(key,message,created_at,updated_at) VALUES(?,?,?,?)
         ON CONFLICT(key) DO UPDATE SET message=excluded.message, updated_at=excluded.updated_at`,
		key, message, now, now,
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
		uuid.New().String(), username, HashPassword(salt, password), salt,
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
