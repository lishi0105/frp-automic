package appsettings

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"frps-status-app.local/status/src/logger"
)

//go:embed default_app_settings.yaml
var defaultAppSettingsYAML []byte

// AppSettingsYAMLPath 为应用层可调参数的唯一配置文件路径（固定，不由环境变量指定）。
const AppSettingsYAMLPath = "/config/app-settings.yml"

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
			b = defaultAppSettingsYAML
			regeneratedFromRead = true
		} else {
			logger.Warn("读取应用配置失败，将尝试写入默认配置: 路径=%q 错误=%v", path, readErr)
			if err := writeDefaultAppSettingsFile(path); err != nil {
				return fmt.Errorf("读取应用配置 %q: %w; 写入默认配置失败: %w", path, readErr, err)
			}
			b = defaultAppSettingsYAML
			regeneratedFromRead = true
		}
	} else if len(bytes.TrimSpace(b)) == 0 {
		if err := writeDefaultAppSettingsFile(path); err != nil {
			return err
		}
		b = defaultAppSettingsYAML
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
		raw, err = unmarshalAppSettingsMap(defaultAppSettingsYAML)
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
	if err := os.WriteFile(path, defaultAppSettingsYAML, 0o600); err != nil {
		return fmt.Errorf("写入默认应用配置 %q: %w", path, err)
	}
	logger.Info("已生成默认应用 YAML 配置（含中文注释）: %s", path)
	return nil
}
