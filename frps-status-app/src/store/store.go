package store

import (
	"database/sql"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"frps-status-app.local/status/src/model"
)

type Store struct {
	db *sql.DB
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
		`CREATE TABLE IF NOT EXISTS event_alert_state (key TEXT PRIMARY KEY, sent_at TEXT NOT NULL)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	_, _ = s.db.Exec(`ALTER TABLE daily_traffic ADD COLUMN peak_conns INTEGER NOT NULL DEFAULT 0`)
	defaults := map[string]string{"alert_in_gb": "0", "alert_out_gb": "0", "alert_total_gb": "0", "smtp_port": "465", "smtp_enabled": "false", "alert_proxy_offline": "false", "alert_cert_expiry": "false", "alert_cert_days": "15"}
	for k, v := range defaults {
		if _, err := s.db.Exec(`INSERT OR IGNORE INTO settings(key,value) VALUES(?,?)`, k, v); err != nil {
			return err
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
	return err
}

func (s *Store) PublicSettings() (model.PublicSettings, error) {
	return model.PublicSettings{
		AlertInGB:       parseFloat(s.Setting("alert_in_gb")),
		AlertOutGB:      parseFloat(s.Setting("alert_out_gb")),
		AlertTotalGB:    parseFloat(s.Setting("alert_total_gb")),
		SMTPHost:        s.Setting("smtp_host"),
		SMTPPort:        int(parseFloatDefault(s.Setting("smtp_port"), 465)),
		SMTPUser:        s.Setting("smtp_user"),
		SMTPFrom:        s.Setting("smtp_from"),
		SMTPTo:          s.Setting("smtp_to"),
		SMTPEnabled:        strings.EqualFold(s.Setting("smtp_enabled"), "true"),
		SMTPAuthCode:       s.Setting("smtp_auth_code"),
		AlertProxyOffline:  strings.EqualFold(s.Setting("alert_proxy_offline"), "true"),
		AlertCertExpiry:    strings.EqualFold(s.Setting("alert_cert_expiry"), "true"),
		AlertCertDays:      int(parseFloatDefault(s.Setting("alert_cert_days"), 15)),
	}, nil
}

func (s *Store) RecordTraffic(proxies []model.ProxyTraffic) error {
	now := time.Now()
	day := now.Format("2006-01-02")
	cutoff := now.AddDate(0, 0, -30).Format("2006-01-02")
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range proxies {
		var lastIn, lastOut uint64
		err := tx.QueryRow(`SELECT last_in,last_out FROM proxy_counters WHERE name=? AND type=?`, p.Name, p.Type).Scan(&lastIn, &lastOut)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = tx.Exec(`INSERT INTO proxy_counters(name,type,last_in,last_out,updated_at) VALUES(?,?,?,?,?)`, p.Name, p.Type, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339))
			if err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		deltaIn, deltaOut := deltaCounter(lastIn, p.CurrentIn), deltaCounter(lastOut, p.CurrentOut)
		_, err = tx.Exec(`INSERT INTO daily_traffic(day,proxy_name,proxy_type,in_bytes,out_bytes,peak_conns) VALUES(?,?,?,?,?,?)
ON CONFLICT(day,proxy_name,proxy_type) DO UPDATE SET in_bytes=in_bytes+excluded.in_bytes,out_bytes=out_bytes+excluded.out_bytes,peak_conns=MAX(daily_traffic.peak_conns,excluded.peak_conns)`, day, p.Name, p.Type, deltaIn, deltaOut, p.CurConns)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE proxy_counters SET last_in=?, last_out=?, updated_at=? WHERE name=? AND type=?`, p.CurrentIn, p.CurrentOut, now.UTC().Format(time.RFC3339), p.Name, p.Type)
		if err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MonthTotals(month string) (uint64, uint64, error) {
	var in, out int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(in_bytes),0), COALESCE(SUM(out_bytes),0) FROM daily_traffic WHERE day LIKE ?`, month+"-%").Scan(&in, &out)
	return uint64(clampZero(in)), uint64(clampZero(out)), err
}

func (s *Store) MonthTotalForProxy(name, typ, month string) (uint64, uint64, error) {
	var in, out int64
	err := s.db.QueryRow(`SELECT COALESCE(SUM(in_bytes),0), COALESCE(SUM(out_bytes),0) FROM daily_traffic WHERE proxy_name=? AND proxy_type=? AND day LIKE ?`, name, typ, month+"-%").Scan(&in, &out)
	return uint64(clampZero(in)), uint64(clampZero(out)), err
}

func (s *Store) AlertSent(month, direction string) bool {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM alert_state WHERE month=? AND direction=?`, month, direction).Scan(&count)
	return count > 0
}

func (s *Store) MarkAlertSent(month, direction string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO alert_state(month,direction,sent_at) VALUES(?,?,?)`, month, direction, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) EventAlertSent(key string) bool {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM event_alert_state WHERE key=?`, key).Scan(&count)
	return count > 0
}

func (s *Store) SetEventAlert(key string) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO event_alert_state(key,sent_at) VALUES(?,?)`, key, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ClearEventAlert(key string) error {
	_, err := s.db.Exec(`DELETE FROM event_alert_state WHERE key=?`, key)
	return err
}

func (s *Store) Vacuum() error {
	_, err := s.db.Exec(`VACUUM`)
	return err
}

func (s *Store) Purge(days int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	res, err := s.db.Exec(`DELETE FROM daily_traffic WHERE day < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) DailyTraffic() ([]map[string]any, error) {
	rows, err := s.db.Query(`SELECT day,proxy_name,proxy_type,in_bytes,out_bytes,peak_conns FROM daily_traffic ORDER BY day,proxy_name,proxy_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var data []map[string]any
	for rows.Next() {
		var day, name, typ string
		var in, out, peak int64
		if err := rows.Scan(&day, &name, &typ, &in, &out, &peak); err != nil {
			return nil, err
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
