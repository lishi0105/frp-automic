package server

import (
	"context"
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

func (a *App) handleSpeedtests(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		a.speedtestsMu.Lock()
		tasks := make([]*speedtestTask, 0, len(a.speedtests))
		for _, task := range a.speedtests {
			copyTask := *task
			tasks = append(tasks, &copyTask)
		}
		running := a.speedtestRunning
		a.speedtestsMu.Unlock()
		writeJSON(w, map[string]any{
			"targets": a.cfg.SpeedtestTargets,
			"tasks":   tasks,
			"running": running,
		})
	case http.MethodPost:
		a.createSpeedtest(w, r)
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
	a.speedtestsMu.Lock()
	task := a.speedtests[id]
	var copyTask *speedtestTask
	if task != nil {
		copied := *task
		copyTask = &copied
	}
	a.speedtestsMu.Unlock()
	if copyTask == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, copyTask)
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
	a.speedtestsMu.Lock()
	if a.speedtestRunning {
		a.speedtestsMu.Unlock()
		writeJSONStatus(w, http.StatusConflict, map[string]any{"ok": false, "error": "已有测速任务正在运行"})
		return
	}
	a.speedtests[task.ID] = task
	a.speedtestRunning = true
	a.speedtestsMu.Unlock()

	go a.runSpeedtest(task.ID)
	writeJSONStatus(w, http.StatusAccepted, task)
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
	a.speedtestsMu.Lock()
	task := a.speedtests[id]
	if task == nil {
		a.speedtestRunning = false
		a.speedtestsMu.Unlock()
		return
	}
	now := time.Now()
	task.Status = "running"
	task.StartedAt = &now
	target := task.Target
	direction := task.Direction
	duration := task.DurationSeconds
	a.speedtestsMu.Unlock()

	result, err := runIperf3(target, direction, duration)

	finished := time.Now()
	a.speedtestsMu.Lock()
	defer a.speedtestsMu.Unlock()
	if task = a.speedtests[id]; task != nil {
		task.FinishedAt = &finished
		if err != nil {
			task.Status = "failed"
			task.Error = err.Error()
			logger.Warn("iperf3 测速失败: target=%s error=%v", target.Name, err)
		} else {
			task.Status = "completed"
			task.Result = result
			logger.Info("iperf3 测速完成: target=%s direction=%s duration=%ds", target.Name, direction, duration)
		}
	}
	a.speedtestRunning = false
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
