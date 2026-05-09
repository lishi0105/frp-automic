package appsettings

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"frps-status-app.local/status/src/logger"
)

const defaultAppSettingsYAML = `# =============================================================================
# FRPS 状态面板 — 应用层可调参数（唯一来源）
# =============================================================================
# 本文件须位于容器内固定路径：/config/app-settings.yaml（由宿主机目录挂载到 ./config）。
# - 若文件不存在或为空：首次启动将自动写入本模板（含注释）再加载。
# - 部署日期 deploy_date 仅保存在 SQLite（/data/frps-status.sqlite 的 app_meta），勿在本文件填写。
# =============================================================================

# --- 月度流量告警阈值（GB）；0 表示该项不参与告警 ---
threshold_in_gb: 0
threshold_out_gb: 0
threshold_total_gb: 0

# --- 月度用量上限/展示用「封顶」（GB）；0 表示不限制 ---
limit_in_gb: 0
limit_out_gb: 0
limit_total_gb: 0

# --- 用量基数修正（GB），用于抵消历史偏移等；一般保持 0 ---
initial_in_gb: 0
initial_out_gb: 0

# --- 历史流量与代理状态等数据的保留天数（1～365，默认 60）---
history_retention_days: 60

# --- 磁盘剩余空间告警阈值（MB）；0 表示关闭该项告警 ---
disk_free_space_alert_threshold_mb: 0

# --- SMTP（告警邮件、忘记密码等）---
smtp_enabled: false
smtp_host: ""
smtp_port: 465
smtp_user: ""
smtp_auth_code: ""
smtp_from: ""
smtp_to: ""

# --- 事件类告警 ---
alert_proxy_offline: false
alert_cert_expiry: false
# 证书剩余天数低于该值时纳入「将到期」预警（1～90）
alert_cert_days: 15
`

// AppSettingsYAMLPath 为应用层可调参数的唯一配置文件路径（固定，不由环境变量指定）。
const AppSettingsYAMLPath = "/config/app-settings.yaml"

// LoadAppSettings 从 AppSettingsYAMLPath 读取 YAML 并合并到 Manager。
//
// 以下情况会写入内置默认文件（含中文注释）后再加载：
//   - 文件不存在；
//   - 文件为空或仅空白；
//   - 打开/读取失败（例如权限短暂异常等，会记录告警后尝试覆盖为默认）；
//   - 内容不是合法 YAML 或顶层不是对象（无法解析为键值）：先将原内容备份为
//     app-settings.yaml.corrupt.<时间戳> 再写入默认。
func LoadAppSettings(m *Manager) error {
	path := AppSettingsYAMLPath

	b, readErr := os.ReadFile(path)
	regeneratedFromRead := false

	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			if err := writeDefaultAppSettingsFile(path); err != nil {
				return err
			}
			b = []byte(defaultAppSettingsYAML)
			regeneratedFromRead = true
		} else {
			logger.Warn("读取应用配置失败，将尝试写入默认配置: 路径=%q 错误=%v", path, readErr)
			if err := writeDefaultAppSettingsFile(path); err != nil {
				return fmt.Errorf("读取应用配置 %q: %w; 写入默认配置失败: %w", path, readErr, err)
			}
			b = []byte(defaultAppSettingsYAML)
			regeneratedFromRead = true
		}
	} else if len(bytes.TrimSpace(b)) == 0 {
		if err := writeDefaultAppSettingsFile(path); err != nil {
			return err
		}
		b = []byte(defaultAppSettingsYAML)
		regeneratedFromRead = true
	}

	raw, err := unmarshalAppSettingsMap(b)
	if err != nil {
		if regeneratedFromRead {
			return fmt.Errorf("内置默认应用配置 YAML 无效: %w", err)
		}
		backup := path + ".corrupt." + time.Now().Format(time.RFC3339Nano)
		if werr := os.WriteFile(backup, b, 0o600); werr != nil {
			logger.Warn("应用配置 YAML 无法解析，备份损坏文件失败: %v", werr)
		} else {
			logger.Warn("应用配置 YAML 无法解析，已备份损坏内容到: %s", backup)
		}
		if werr := writeDefaultAppSettingsFile(path); werr != nil {
			return fmt.Errorf("解析应用配置 %q: %w; 写入默认配置失败: %w", path, err, werr)
		}
		raw, err = unmarshalAppSettingsMap([]byte(defaultAppSettingsYAML))
		if err != nil {
			return fmt.Errorf("内置默认应用配置 YAML 无效: %w", err)
		}
	}

	_ = m.ApplyPOST(raw)
	return nil
}

func unmarshalAppSettingsMap(data []byte) (map[string]any, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, fmt.Errorf("顶层为 null，需要 YAML 映射（键值对）")
	}
	return raw, nil
}

func writeDefaultAppSettingsFile(path string) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建应用配置目录 %q: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, []byte(defaultAppSettingsYAML), 0o600); err != nil {
		return fmt.Errorf("写入默认应用配置 %q: %w", path, err)
	}
	logger.Info("已生成默认应用 YAML 配置（含中文注释）: %s", path)
	return nil
}
