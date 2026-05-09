package server

import (
	"fmt"
	"math"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"frps-status-app.local/status/src/config"
	"frps-status-app.local/status/src/diskmon"
	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/mail"
	"frps-status-app.local/status/src/model"
)

const (
	diskAlertEmailCooldown = 24 * time.Hour
	minDiskThresholdFloor  = uint64(1024 * 1024) // 1 MiB，阈值折算后的字节下限
	bytesPerMiB            = uint64(1024 * 1024)
)

func computeInitialThresholdMBFromFree(free uint64) uint64 {
	tb := free * 20 / 100
	if tb < minDiskThresholdFloor {
		tb = minDiskThresholdFloor
	}
	mb := tb / bytesPerMiB
	if mb < 1 {
		mb = 1
	}
	return mb
}

func thresholdBytesFromMB(mb uint64) uint64 {
	if mb == 0 {
		return minDiskThresholdFloor
	}
	b := mb * bytesPerMiB
	if b < minDiskThresholdFloor {
		return minDiskThresholdFloor
	}
	return b
}

// diskAlertState 磁盘危急邮件冷却（与快照锁分离，避免与 Refresh 争用）。
type diskAlertState struct {
	mu          sync.Mutex
	lastEmailAt time.Time
}

func (a *App) effectiveLogDir() string {
	if s := strings.TrimSpace(a.cfg.LogDir); s != "" {
		return s
	}
	return config.LogDirFixed
}

func (a *App) checkDiskSpaceRoutine() {
	dataDir := filepath.Dir(a.cfg.DBPath)
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	free, total, err := diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if err != nil {
		logger.Error("磁盘空间定期检测失败：无法读取分区信息 数据目录=%s 错误=%v", dataDir, err)
		return
	}

	logDir := a.effectiveLogDir()
	logSize := diskmon.LogDirLogFilesSize(logDir)
	dbSize := diskmon.SQLiteBundleSize(a.cfg.DBPath)

	settings := a.appcfg.PublicSettings()

	mb := settings.DiskFreeSpaceAlertThresholdMB
	if mb == 0 {
		mb = computeInitialThresholdMBFromFree(free)
		a.appcfg.SetDiskFreeSpaceAlertThresholdMB(mb)
		logger.Info("磁盘告警阈值首次初始化：按当前分区剩余 20%% 折算并写入进程内配置（单位 MB，1MB=1024² 字节，且不低于 1MiB）数据目录=%s 当前剩余=%s 阈值=%d MB 约合=%s",
			dataDir, mail.HumanBytes(free), mb, mail.HumanBytes(mb*bytesPerMiB))
	}
	thresholdBytes := thresholdBytesFromMB(mb)

	logger.Info("磁盘空间定期检测 数据目录=%s 分区总容量=%s 分区剩余=%s 日志目录=%s 日志文件合计=%s 数据库估算占用=%s 告警阈值=%d MB(约合 %s，剩余须不低于)",
		dataDir, mail.HumanBytes(total), mail.HumanBytes(free), logDir, u64Human(logSize), u64Human(dbSize), mb, mail.HumanBytes(thresholdBytes))

	if free >= thresholdBytes {
		_ = a.store.ClearWarning("disk_space_critical")
		logger.Info("磁盘空间定期检测结论：剩余空间不低于告警阈值，无需应急清理")
		return
	}

	logger.Warn("磁盘空间定期检测结论：剩余空间低于告警阈值，启动应急清理流程 剩余=%s 阈值=%d MB(约合 %s)", mail.HumanBytes(free), mb, mail.HumanBytes(thresholdBytes))

	removed, freed, err := logger.DiskPressureRemoveNonCurrentLogs()
	if err != nil {
		logger.Error("磁盘应急步骤「删除非当日日志文件」失败 错误=%v", err)
	} else {
		logger.Info("磁盘应急步骤「删除非当日日志文件」完成 删除文件数=%d 释放日志体积约=%s", removed, u64Human(freed))
	}
	free, _, _ = diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if free >= thresholdBytes {
		logger.Info("删除旧日志文件后，磁盘剩余空间已恢复至阈值以上 剩余=%s", mail.HumanBytes(free))
		_ = a.store.ClearWarning("disk_space_critical")
		return
	}
	logger.Warn("删除旧日志文件后，剩余空间仍低于阈值 剩余=%s 阈值=%d MB(约合 %s)", mail.HumanBytes(free), mb, mail.HumanBytes(thresholdBytes))

	if err := logger.DiskPressureRotateCurrentLog(); err != nil {
		logger.Error("磁盘应急步骤「清空当日日志并重新打开」失败 错误=%v", err)
	} else {
		logger.Info("磁盘应急步骤「清空当日日志并重新打开」完成，用于截断可能异常膨胀的当前日志文件")
	}
	removed2, freed2, err := logger.DiskPressureRemoveNonCurrentLogs()
	if err != nil {
		logger.Error("磁盘应急步骤「再次删除非当日日志文件」失败 错误=%v", err)
	} else {
		logger.Info("磁盘应急步骤「再次删除非当日日志文件」完成 删除文件数=%d 释放约=%s", removed2, u64Human(freed2))
	}
	free, _, _ = diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if free >= thresholdBytes {
		logger.Info("日志轮转与二次清理后，磁盘剩余空间已不低于阈值 剩余=%s", mail.HumanBytes(free))
		_ = a.store.ClearWarning("disk_space_critical")
		return
	}
	logger.Warn("日志轮转与二次清理后，剩余空间仍低于阈值 剩余=%s 阈值=%d MB(约合 %s)", mail.HumanBytes(free), mb, mail.HumanBytes(thresholdBytes))

	dbSizeBefore := diskmon.SQLiteBundleSize(a.cfg.DBPath)
	logger.Info("磁盘应急步骤「SQLite VACUUM 精简数据库」开始 精简前数据库估算占用=%s", u64Human(dbSizeBefore))
	if err := a.store.Vacuum(); err != nil {
		logger.Error("磁盘应急步骤「SQLite VACUUM」执行失败 错误=%v", err)
	} else {
		dbSizeAfter := diskmon.SQLiteBundleSize(a.cfg.DBPath)
		reclaimed := dbSizeBefore - dbSizeAfter
		if reclaimed < 0 {
			reclaimed = 0
		}
		logger.Info("磁盘应急步骤「SQLite VACUUM」完成 精简后数据库估算占用=%s 估算减少=%s",
			u64Human(dbSizeAfter), u64Human(reclaimed))
	}
	free, _, _ = diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if free >= thresholdBytes {
		logger.Info("数据库精简后，磁盘剩余空间已不低于阈值 剩余=%s", mail.HumanBytes(free))
		_ = a.store.ClearWarning("disk_space_critical")
		return
	}
	logger.Warn("数据库精简后，剩余空间仍低于阈值 剩余=%s 阈值=%d MB(约合 %s)", mail.HumanBytes(free), mb, mail.HumanBytes(thresholdBytes))

	for round := 0; round < 1000 && free < thresholdBytes; round++ {
		nDays, err := a.store.TrafficDistinctDayCount()
		if err != nil {
			logger.Error("磁盘应急步骤「按日轮删流量」统计剩余天数失败 错误=%v", err)
			break
		}
		if nDays <= 2 {
			logger.Warn("磁盘应急步骤「按日轮删流量」终止：历史流量已仅剩不超过两个自然日的数据，不再继续删除")
			break
		}
		oldest, err := a.store.OldestTrafficDay()
		if err != nil || oldest == "" {
			if err != nil {
				logger.Error("磁盘应急步骤「按日轮删流量」查询最早日期失败 错误=%v", err)
			} else {
				logger.Warn("磁盘应急步骤「按日轮删流量」终止：无流量日表数据可删")
			}
			break
		}
		t0, err := time.ParseInLocation("2006-01-02", oldest, time.Local)
		if err != nil {
			logger.Error("磁盘应急步骤「按日轮删流量」解析最早日期失败 日期=%s 错误=%v", oldest, err)
			break
		}
		cutoff := t0.AddDate(0, 0, 10).Format("2006-01-02")
		logger.Info("磁盘应急步骤「按十日轮回删除流量历史」第 %d 轮：将删除日期早于 %s 的记录（从最早日期起约 10 个自然日）", round+1, cutoff)
		deletedRows, err := a.store.PurgeTrafficDayBefore(cutoff)
		if err != nil {
			logger.Error("磁盘应急步骤「按十日轮回删除流量历史」执行 SQL 失败 错误=%v", err)
			break
		}
		logger.Info("磁盘应急步骤「按十日轮回删除流量历史」第 %d 轮完成 删除行数合计=%d", round+1, deletedRows)
		if deletedRows == 0 {
			logger.Warn("磁盘应急步骤「按十日轮回删除流量历史」本轮未删除任何行，停止轮回以防死循环")
			break
		}
		free, _, _ = diskmon.FreeAndTotalBytes(a.cfg.DBPath)
		logger.Info("磁盘应急步骤「按十日轮回删除流量历史」后轮询分区剩余=%s", mail.HumanBytes(free))
		if free >= thresholdBytes {
			logger.Info("按日删除流量历史后，磁盘剩余空间已不低于阈值")
			_ = a.store.ClearWarning("disk_space_critical")
			return
		}
	}

	free, _, _ = diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if free >= thresholdBytes {
		_ = a.store.ClearWarning("disk_space_critical")
		logger.Info("磁盘应急清理结束：剩余空间已恢复至阈值以上 剩余=%s", mail.HumanBytes(free))
		return
	}

	msg := fmt.Sprintf("数据目录所在分区在依次清理旧日志、轮转当日日志、执行 VACUUM、并尽量按十日删除流量历史后，剩余空间 %s 仍低于配置阈值 %d MB（约合 %s），请尽快扩容或迁移数据。", mail.HumanBytes(free), mb, mail.HumanBytes(thresholdBytes))
	logger.Error("磁盘空间严重不足：%s", msg)
	_ = a.store.SetWarning("disk_space_critical", msg)
	subject := "FRPS 状态监控 - 磁盘空间严重不足告警"
	body := msg + "\n\n（本邮件在 24 小时内最多发送一次。）"
	a.maybeSendDiskCriticalEmail(settings, subject, body)
}

func (a *App) maybeSendDiskCriticalEmail(settings model.PublicSettings, subject, body string) {
	if !settings.SMTPEnabled {
		logger.Warn("磁盘空间危急：未启用邮件告警开关，已跳过发送邮件（仍已写入后台告警）")
		return
	}
	if !a.appcfg.SMTPVerified() {
		logger.Warn("磁盘空间危急：SMTP 尚未通过测试邮件验证，已跳过发送邮件（仍已写入后台告警）")
		return
	}
	if settings.SMTPAuthCode == "" || settings.SMTPHost == "" || settings.SMTPFrom == "" || settings.SMTPTo == "" {
		logger.Warn("磁盘空间危急：SMTP 配置不完整，已跳过发送邮件（仍已写入后台告警）")
		return
	}
	a.diskAlert.mu.Lock()
	defer a.diskAlert.mu.Unlock()
	if !a.diskAlert.lastEmailAt.IsZero() && time.Since(a.diskAlert.lastEmailAt) < diskAlertEmailCooldown {
		logger.Info("磁盘空间危急告警邮件已在冷却期内发送过，本次不再重复发信")
		return
	}
	if err := mail.Send(settings, settings.SMTPAuthCode, subject, body); err != nil {
		logger.Error("磁盘空间危急告警邮件发送失败 错误=%v", err)
		return
	}
	a.diskAlert.lastEmailAt = time.Now()
	logger.Info("磁盘空间危急告警邮件发送成功 主题=%s", subject)
}

func u64Human(v int64) string {
	if v < 0 {
		v = 0
	}
	return mail.HumanBytes(uint64(v))
}

// bytesToBinaryMB 将字节数转为二进制 MB（1 MB = 1024² 字节），保留两位小数。
func bytesToBinaryMB(v int64) float64 {
	if v <= 0 {
		return 0
	}
	return math.Round(float64(v)/1024/1024*100) / 100
}

func (a *App) handleStorageAppUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("程序占用空间请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logDir := a.effectiveLogDir()
	logBytes := diskmon.LogDirLogFilesSize(logDir)
	dataBytes := diskmon.SQLiteBundleSize(a.cfg.DBPath)
	logMB := bytesToBinaryMB(logBytes)
	dataMB := bytesToBinaryMB(dataBytes)
	totalMB := math.Round((logMB+dataMB)*100) / 100
	logger.Info("程序占用空间已读取 来源=%s 日志=%.2f MB 数据=%.2f MB 合计=%.2f MB", r.RemoteAddr, logMB, dataMB, totalMB)
	writeJSON(w, map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"log_mb":       logMB,
		"data_mb":      dataMB,
		"total_mb":     totalMB,
	})
}

func (a *App) handleStorageCleanup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		logger.Warn("手动存储清理请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.storageOpsMu.Lock()
	defer a.storageOpsMu.Unlock()
	logger.Info("手动存储清理开始：已获取与定时任务相同的互斥锁，避免与自动历史清理/磁盘应急流程并发 来源=%s", r.RemoteAddr)

	out := map[string]any{"ok": true}
	removed, freed, err := logger.DiskPressureRemoveNonCurrentLogs()
	if err != nil {
		logger.Error("手动存储清理步骤「删除非当日日志文件」失败 错误=%v", err)
		out["ok"] = false
		out["log_error"] = err.Error()
	} else {
		logger.Info("手动存储清理步骤「删除非当日日志文件」完成 删除文件数=%d 释放约=%s", removed, mail.HumanBytes(uint64(freed)))
		out["log_files_removed"] = removed
		out["log_bytes_freed"] = freed
	}

	settings := a.appcfg.PublicSettings()
	days := settings.HistoryRetentionDays
	if days < 1 {
		days = 60
	}
	deleted, err := a.store.Purge(days)
	if err != nil {
		logger.Error("手动存储清理步骤「按保留天数清理流量历史」失败 保留天数=%d 错误=%v", days, err)
		out["ok"] = false
		out["purge_error"] = err.Error()
	} else {
		logger.Info("手动存储清理步骤「按保留天数清理流量历史」完成 保留天数=%d 删除行数合计=%d", days, deleted)
		out["traffic_rows_deleted"] = deleted
		out["history_retention_days"] = days
	}

	if err := a.store.Vacuum(); err != nil {
		logger.Error("手动存储清理步骤「SQLite VACUUM」失败 错误=%v", err)
		out["ok"] = false
		out["vacuum_error"] = err.Error()
	} else {
		logger.Info("手动存储清理步骤「SQLite VACUUM」完成")
		out["vacuum_ok"] = true
	}

	logger.Info("手动存储清理流程结束 来源=%s 成功=%t", r.RemoteAddr, out["ok"])
	writeJSON(w, out)
}

func (a *App) handleStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logger.Warn("存储盘信息请求被拒绝：请求方法不允许 方法=%s", r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	logger.Info("存储盘使用信息已读取 来源=%s", r.RemoteAddr)
	writeJSON(w, a.collectStorageDiskInfo())
}

func (a *App) collectStorageDiskInfo() map[string]any {
	logDir := a.effectiveLogDir()
	out := map[string]any{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"ok":           false,
		"db_path":      a.cfg.DBPath,
		"log_dir":      logDir,
		"log_files_bytes": diskmon.LogDirLogFilesSize(logDir),
		"database_bytes":  diskmon.SQLiteBundleSize(a.cfg.DBPath),
	}
	dataDir := filepath.Dir(a.cfg.DBPath)
	if abs, err := filepath.Abs(dataDir); err == nil {
		dataDir = abs
	}
	out["data_dir"] = dataDir

	free, total, err := diskmon.FreeAndTotalBytes(a.cfg.DBPath)
	if err != nil {
		out["error"] = err.Error()
		return out
	}
	var used uint64
	if total >= free {
		used = total - free
	}
	var usagePct float64
	if total > 0 {
		usagePct = float64(used) * 100.0 / float64(total)
	}
	out["partition"] = map[string]any{
		"total_bytes":   total,
		"free_bytes":    free,
		"used_bytes":    used,
		"usage_percent": usagePct,
	}

	settings := a.appcfg.PublicSettings()
	mbSaved := settings.DiskFreeSpaceAlertThresholdMB
	var thresholdBytes uint64
	var effMB uint64
	usesSaved := mbSaved > 0
	if usesSaved {
		effMB = mbSaved
		thresholdBytes = thresholdBytesFromMB(mbSaved)
	} else {
		effMB = computeInitialThresholdMBFromFree(free)
		thresholdBytes = thresholdBytesFromMB(effMB)
	}
	out["ok"] = true
	out["disk_alert_threshold_mb_saved"] = mbSaved
	out["disk_alert_threshold_mb_effective"] = effMB
	out["disk_alert_threshold_bytes"] = thresholdBytes
	out["disk_alert_threshold_is_preview"] = !usesSaved
	out["below_disk_alert_threshold"] = free < thresholdBytes
	return out
}
