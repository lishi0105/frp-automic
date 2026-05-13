package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/google/uuid"

	"frps-status-app.local/status/src/config"
	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/store"
)

type speedtestTask struct {
	ID              string                 `json:"id"`
	Target          config.SpeedtestTarget `json:"target"`
	Direction       string                 `json:"direction"`
	DurationSeconds int                    `json:"duration_seconds"`
	Status          string                 `json:"status"`
	Error           string                 `json:"error,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	StartedAt       *time.Time             `json:"started_at,omitempty"`
	FinishedAt      *time.Time             `json:"finished_at,omitempty"`
	Result          *speedtestResult       `json:"result,omitempty"`
}

type speedtestResult struct {
	SentMbps     float64        `json:"sent_mbps"`
	ReceivedMbps float64        `json:"received_mbps"`
	Retransmits  int64          `json:"retransmits"`
	Raw          map[string]any `json:"raw,omitempty"`
}

type speedtestCreateRequest struct {
	Target          string `json:"target"`
	Direction       string `json:"direction"`
	DurationSeconds int    `json:"duration_seconds"`
}

type speedtestCleanupRequest struct {
	KeepLatest int `json:"keep_latest"`
}

func (a *App) handleSpeedtests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		records, err := a.store.ListSpeedtestTasks(50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tasks := make([]*speedtestTask, 0, len(records))
		for _, record := range records {
			tasks = append(tasks, taskFromRecord(record))
		}
		running, err := a.store.HasRunningSpeedtestTask()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{
			"targets": a.cfg.SpeedtestTargets,
			"tasks":   tasks,
			"running": running,
		})
	case http.MethodPost:
		a.createSpeedtest(w, r)
	case http.MethodDelete:
		a.cleanupSpeedtests(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleSpeedtestTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/speedtests/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	record, err := a.store.GetSpeedtestTask(id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, taskFromRecord(record))
}

func (a *App) createSpeedtest(w http.ResponseWriter, r *http.Request) {
	if len(a.cfg.SpeedtestTargets) == 0 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "未配置测速目标"})
		return
	}
	var req speedtestCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	target, ok := a.findSpeedtestTarget(req.Target)
	if !ok {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "测速目标不存在"})
		return
	}
	direction := strings.ToLower(strings.TrimSpace(req.Direction))
	if direction == "" {
		direction = "forward"
	}
	if direction != "forward" && direction != "reverse" {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "direction 只支持 forward 或 reverse"})
		return
	}
	duration := req.DurationSeconds
	if duration <= 0 {
		duration = 10
	}
	if duration < 3 || duration > 60 {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "duration_seconds 必须在 3 到 60 之间"})
		return
	}

	task := &speedtestTask{
		ID:              uuid.NewString(),
		Target:          target,
		Direction:       direction,
		DurationSeconds: duration,
		Status:          "queued",
		CreatedAt:       time.Now(),
	}
	running, err := a.store.HasRunningSpeedtestTask()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if running {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"ok": false, "error": "已有测速任务正在运行"})
		return
	}
	if err := a.store.CreateSpeedtestTask(recordFromTask(task)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	go a.runSpeedtest(task.ID)
	writeJSONStatus(w, http.StatusAccepted, task)
}

func (a *App) cleanupSpeedtests(w http.ResponseWriter, r *http.Request) {
	running, err := a.store.HasRunningSpeedtestTask()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if running {
		writeJSONStatus(w, http.StatusConflict, map[string]any{"ok": false, "error": "测速任务运行中，暂不允许清理"})
		return
	}
	var in speedtestCleanupRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&in)
	}
	var deleted int64
	if in.KeepLatest > 0 {
		deleted, err = a.store.DeleteSpeedtestTasksKeepLatest(in.KeepLatest)
	} else {
		deleted, err = a.store.DeleteAllSpeedtestTasks()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "deleted": deleted, "keep_latest": in.KeepLatest})
}

func (a *App) findSpeedtestTarget(name string) (config.SpeedtestTarget, bool) {
	name = strings.TrimSpace(name)
	if name == "" && len(a.cfg.SpeedtestTargets) == 1 {
		return a.cfg.SpeedtestTargets[0], true
	}
	for _, target := range a.cfg.SpeedtestTargets {
		if target.Name == name {
			return target, true
		}
	}
	return config.SpeedtestTarget{}, false
}

func (a *App) runSpeedtest(id string) {
	record, err := a.store.GetSpeedtestTask(id)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		logger.Error("读取测速任务失败: id=%s err=%v", id, err)
		return
	}
	now := time.Now()
	if err := a.store.UpdateSpeedtestTaskStarted(id, now.UTC().Format(time.RFC3339)); err != nil {
		logger.Error("更新测速任务为 running 失败: id=%s err=%v", id, err)
		return
	}
	target := config.SpeedtestTarget{Name: record.TargetName, Host: record.TargetHost, Port: record.TargetPort}
	direction := record.Direction
	duration := record.DurationSeconds

	result, err := runIperf3(target, direction, duration)

	finished := time.Now()
	if err != nil {
		if dbErr := a.store.FinishSpeedtestTask(id, "failed", err.Error(), finished.UTC().Format(time.RFC3339), 0, 0, 0, ""); dbErr != nil {
			logger.Error("写入测速失败结果失败: id=%s err=%v", id, dbErr)
		}
		logger.Warn("iperf3 测速失败: target=%s error=%v", target.Name, err)
		return
	}
	rawJSON, _ := json.Marshal(result.Raw)
	if dbErr := a.store.FinishSpeedtestTask(id, "completed", "", finished.UTC().Format(time.RFC3339), result.SentMbps, result.ReceivedMbps, result.Retransmits, string(rawJSON)); dbErr != nil {
		logger.Error("写入测速成功结果失败: id=%s err=%v", id, dbErr)
		return
	}
	logger.Info("iperf3 测速完成: target=%s direction=%s duration=%ds", target.Name, direction, duration)
}

func runIperf3(target config.SpeedtestTarget, direction string, duration int) (*speedtestResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(duration+15)*time.Second)
	defer cancel()
	args := []string{"-J", "-c", target.Host, "-p", fmt.Sprint(target.Port), "-t", fmt.Sprint(duration)}
	if direction == "reverse" {
		args = append(args, "-R")
	}
	cmd := exec.CommandContext(ctx, "iperf3", args...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return nil, errors.New("iperf3 执行超时")
	}
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("iperf3 执行失败：%s", msg)
	}
	var raw map[string]any
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("解析 iperf3 JSON 失败：%w", err)
	}
	return summarizeIperf(raw), nil
}

func summarizeIperf(raw map[string]any) *speedtestResult {
	end, _ := raw["end"].(map[string]any)
	sent, _ := end["sum_sent"].(map[string]any)
	received, _ := end["sum_received"].(map[string]any)
	return &speedtestResult{
		SentMbps:     bitsPerSecondMbps(sent),
		ReceivedMbps: bitsPerSecondMbps(received),
		Retransmits:  int64(numberValue(sent["retransmits"])),
		Raw:          raw,
	}
}

func bitsPerSecondMbps(m map[string]any) float64 {
	return numberValue(m["bits_per_second"]) / 1000 / 1000
}

func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}

func recordFromTask(task *speedtestTask) store.SpeedtestTaskRecord {
	return store.SpeedtestTaskRecord{
		ID:              task.ID,
		TargetName:      task.Target.Name,
		TargetHost:      task.Target.Host,
		TargetPort:      task.Target.Port,
		Direction:       task.Direction,
		DurationSeconds: task.DurationSeconds,
		Status:          task.Status,
		ErrorMsg:        task.Error,
		CreatedAt:       task.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func taskFromRecord(record store.SpeedtestTaskRecord) *speedtestTask {
	task := &speedtestTask{
		ID: record.ID,
		Target: config.SpeedtestTarget{
			Name: record.TargetName,
			Host: record.TargetHost,
			Port: record.TargetPort,
		},
		Direction:       record.Direction,
		DurationSeconds: record.DurationSeconds,
		Status:          record.Status,
		Error:           record.ErrorMsg,
		CreatedAt:       parseRFC3339OrZero(record.CreatedAt),
	}
	if record.StartedAt != "" {
		t := parseRFC3339OrZero(record.StartedAt)
		task.StartedAt = &t
	}
	if record.FinishedAt != "" {
		t := parseRFC3339OrZero(record.FinishedAt)
		task.FinishedAt = &t
	}
	if record.Status == "completed" || record.RawJSON != "" {
		task.Result = &speedtestResult{
			SentMbps:     record.SentMbps,
			ReceivedMbps: record.ReceivedMbps,
			Retransmits:  record.Retransmits,
		}
		if strings.TrimSpace(record.RawJSON) != "" {
			var raw map[string]any
			if err := json.Unmarshal([]byte(record.RawJSON), &raw); err == nil {
				task.Result.Raw = raw
			}
		}
	}
	return task
}

func parseRFC3339OrZero(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
