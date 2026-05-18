package appsettings

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"frps-status-app.local/status/src/logger"
)

const defaultAppSettingsYAML = `# =============================================================================
# FRPS 状态面板 — 应用层可调参数（唯一来源）
# =============================================================================
# 本文件须位于容器内固定路径：/config/app-settings.yaml（由宿主机目录挂载到 ./config）。
# - 若文件不存在或为空：首次启动将自动写入本模板再加载。
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

# --- 流量统计计费周期起始日（1～31）；0 表示按部署日期自动 ---
traffic_cycle_start_day: 0

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

const encryptedValuePrefix = "enc:v1:"

var appSettingsSecretKey = sha256.Sum256([]byte("frps-status-app:CucrVNoWkrE72GNPZsXn0Yq+8="))

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
				errMsg := fmt.Sprintf("读取应用配置失败，且写入默认配置失败: %v", err)
				logger.Error(errMsg)
				return errors.New(errMsg + "，请检查容器内 /config 目录的权限和挂载情况")
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
			errMsg := fmt.Sprintf("应用配置 YAML 无法解析，且写入默认配置失败: %v", werr)
			logger.Error(errMsg)
			return errors.New(errMsg + "，请检查容器内 /config 目录的权限和挂载情况")
		}
		raw, err = unmarshalAppSettingsMap([]byte(defaultAppSettingsYAML))
		if err != nil {
			errMsg := fmt.Sprintf("内置默认应用配置 YAML 无效，且已尝试写入覆盖: %v", err)
			logger.Error(errMsg)
			return errors.New(errMsg)
		}
	}
	if err := decryptAppSettingsSecrets(raw); err != nil {
		return err
	}
	logger.Info("已加载应用配置")
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
			errMsg := fmt.Sprintf("创建应用配置目录 %q 失败", dir)
			if errors.Is(err, os.ErrPermission) {
				errMsg = fmt.Sprintf("创建应用配置目录 %q 失败: 权限不足", dir)
				logger.Warn(errMsg)
			} else {
				errMsg = fmt.Sprintf("创建应用配置目录 %q 失败: %v", dir, err)
				logger.Warn(errMsg)
			}
			return errors.New(errMsg + "，请检查容器内 /config 目录的权限和挂载情况")
		}
	}
	if err := os.WriteFile(path, []byte(defaultAppSettingsYAML), 0o600); err != nil {
		errMsg := "写入默认应用配置失败"
		if errors.Is(err, os.ErrPermission) {
			errMsg += "（权限不足）"
		} else {
			errMsg += fmt.Sprintf(": %v", err)
			logger.Warn("%s: %v", errMsg, err)
			errMsg += "，请检查容器内 /config 目录的权限和挂载情况"
		}
		return errors.New(errMsg)
	}
	logger.Info("已生成默认应用 YAML 配置: %s", path)
	return nil
}

// SaveAppSettings 将当前进程内配置写回 AppSettingsYAMLPath。
// DeployDate 属于数据库 app_meta，不写入 YAML。
func SaveAppSettings(m *Manager) error {
	if m == nil {
		return fmt.Errorf("应用配置管理器为空")
	}

	m.mu.RLock()
	s := m.data
	m.mu.RUnlock()
	s.normalize()

	content, err := renderAppSettingsYAML(s)
	if err != nil {
		return err
	}
	if err := writeAppSettingsFileAtomic(AppSettingsYAMLPath, []byte(content)); err != nil {
		return err
	}
	logger.Info("应用配置已写入 YAML: %s", AppSettingsYAMLPath)
	return nil
}

func renderAppSettingsYAML(s state) (string, error) {
	smtpHost, err := yamlScalar(s.SMTPHost)
	if err != nil {
		return "", err
	}
	smtpUser, err := yamlScalar(s.SMTPUser)
	if err != nil {
		return "", err
	}
	encryptedSMTPAuthCode := ""
	if s.SMTPAuthCode != "" {
		encryptedSMTPAuthCode, err = encryptSecret(s.SMTPAuthCode)
		if err != nil {
			return "", err
		}
	}
	smtpAuthCode, err := yamlScalar(encryptedSMTPAuthCode)
	if err != nil {
		return "", err
	}
	smtpFrom, err := yamlScalar(s.SMTPFrom)
	if err != nil {
		return "", err
	}
	smtpTo, err := yamlScalar(s.SMTPTo)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`# =============================================================================
# FRPS 状态面板 — 应用层可调参数（唯一来源）
# =============================================================================
# 本文件须位于容器内固定路径：/config/app-settings.yaml（由宿主机目录挂载到 ./config）。
# - 若文件不存在或为空：首次启动将自动写入本模板再加载。
# =============================================================================

# --- 月度流量告警阈值（GB）；0 表示该项不参与告警 ---
threshold_in_gb: %s
threshold_out_gb: %s
threshold_total_gb: %s

# --- 月度用量上限/展示用「封顶」（GB）；0 表示不限制 ---
limit_in_gb: %s
limit_out_gb: %s
limit_total_gb: %s

# --- 用量基数修正（GB），用于抵消历史偏移等；一般保持 0 ---
initial_in_gb: %s
initial_out_gb: %s

# --- 流量统计计费周期起始日（1～31）；0 表示按部署日期自动 ---
traffic_cycle_start_day: %d

# --- 历史流量与代理状态等数据的保留天数（1～365，默认 60）---
history_retention_days: %d

# --- 磁盘剩余空间告警阈值（MB）；0 表示关闭该项告警 ---
disk_free_space_alert_threshold_mb: %d

# --- SMTP（告警邮件、忘记密码等）---
smtp_enabled: %t
smtp_host: %s
smtp_port: %d
smtp_user: %s
smtp_auth_code: %s
smtp_from: %s
smtp_to: %s

# --- 事件类告警 ---
alert_proxy_offline: %t
alert_cert_expiry: %t
# 证书剩余天数低于该值时纳入「将到期」预警（1～90）
alert_cert_days: %d
`,
		formatFloat(s.ThresholdInGB),
		formatFloat(s.ThresholdOutGB),
		formatFloat(s.ThresholdTotalGB),
		formatFloat(s.LimitInGB),
		formatFloat(s.LimitOutGB),
		formatFloat(s.LimitTotalGB),
		formatFloat(s.InitialInGB),
		formatFloat(s.InitialOutGB),
		s.TrafficCycleStartDay,
		s.HistoryRetentionDays,
		s.DiskFreeSpaceAlertThresholdMB,
		s.SMTPEnabled,
		smtpHost,
		s.SMTPPort,
		smtpUser,
		smtpAuthCode,
		smtpFrom,
		smtpTo,
		s.AlertProxyOffline,
		s.AlertCertExpiry,
		s.AlertCertDays,
	), nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func yamlScalar(v string) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decryptAppSettingsSecrets(raw map[string]any) error {
	value, ok := raw["smtp_auth_code"]
	if !ok {
		return nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return nil
	}
	plain, err := decryptSecret(text)
	if err != nil {
		return fmt.Errorf("读取 SMTP 授权码失败：smtp_auth_code 必须是 %s 开头的加密值: %w", encryptedValuePrefix, err)
	}
	raw["smtp_auth_code"] = plain
	return nil
}

func encryptSecret(plain string) (string, error) {
	block, err := aes.NewCipher(appSettingsSecretKey[:])
	if err != nil {
		return "", fmt.Errorf("初始化配置加密失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("初始化配置加密模式失败: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("生成配置加密随机数失败: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return encryptedValuePrefix + base64.RawURLEncoding.EncodeToString(payload), nil
}

func decryptSecret(encoded string) (string, error) {
	if !strings.HasPrefix(encoded, encryptedValuePrefix) {
		return "", fmt.Errorf("不是加密值")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, encryptedValuePrefix))
	if err != nil {
		return "", fmt.Errorf("密文不是有效 base64url: %w", err)
	}
	block, err := aes.NewCipher(appSettingsSecretKey[:])
	if err != nil {
		return "", fmt.Errorf("初始化配置解密失败: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("初始化配置解密模式失败: %w", err)
	}
	if len(payload) <= gcm.NonceSize() {
		return "", fmt.Errorf("密文长度无效")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败: %w", err)
	}
	return string(plain), nil
}

func writeAppSettingsFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建应用配置目录 %q 失败: %w", dir, err)
		}
	}

	tmp, err := os.CreateTemp(dir, ".app-settings-*.tmp")
	if err != nil {
		return fmt.Errorf("创建应用配置临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("写入应用配置临时文件失败: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("设置应用配置临时文件权限失败: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭应用配置临时文件失败: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("替换应用配置文件失败: %w", err)
	}
	return nil
}
