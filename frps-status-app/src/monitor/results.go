package monitor

import (
	"fmt"

	"frps-status-app.local/status/src/mail"
	"frps-status-app.local/status/src/model"
)

type Result struct {
	// LinkProbed 为 true 时，按 LinkHealth 判断检测链路是否可用（父级优先级 1）；为 false 时跳过链路门控（兼容旧调用方/单测）。
	LinkProbed      bool
	LinkHealth      LinkHealth
	HostPressure    HostPressure
	ProxyFetchError string
	Proxies         []model.ProxyTraffic
	Certificates    []model.CertStatus
	Traffic         TrafficResult
	CertThreshold   int
}

// LinkChainOK 在未探测时视为可用；已探测时要求 DNS 与外网均成功。
func (r Result) LinkChainOK() bool {
	if !r.LinkProbed {
		return true
	}
	return r.LinkHealth.OK()
}

type TrafficResult struct {
	Month        string
	MonthIn      uint64
	MonthOut     uint64
	Thresholds   []TrafficThreshold
	LimitReached []TrafficThreshold
}

type TrafficThreshold struct {
	Fingerprint string
	Label       string
	Current     uint64
	ThresholdGB float64
}

func BuildTrafficResult(settings model.PublicSettings, month string, monthIn, monthOut uint64) TrafficResult {
	total := monthIn + monthOut
	out := TrafficResult{Month: month, MonthIn: monthIn, MonthOut: monthOut}
	if settings.ThresholdInGB > 0 && monthIn >= mail.GBToBytes(settings.ThresholdInGB) {
		out.Thresholds = append(out.Thresholds, TrafficThreshold{"traffic_threshold_in", "入站阈值", monthIn, settings.ThresholdInGB})
	}
	if settings.ThresholdOutGB > 0 && monthOut >= mail.GBToBytes(settings.ThresholdOutGB) {
		out.Thresholds = append(out.Thresholds, TrafficThreshold{"traffic_threshold_out", "出站阈值", monthOut, settings.ThresholdOutGB})
	}
	if settings.ThresholdTotalGB > 0 && settings.ThresholdInGB > 0 && settings.ThresholdOutGB > 0 && total >= mail.GBToBytes(settings.ThresholdTotalGB) {
		out.Thresholds = append(out.Thresholds, TrafficThreshold{"traffic_threshold_total", "总量阈值", total, settings.ThresholdTotalGB})
	}
	if settings.LimitInGB > 0 && monthIn >= mail.GBToBytes(settings.LimitInGB) {
		out.LimitReached = append(out.LimitReached, TrafficThreshold{"traffic_limit_in", "入站限额", monthIn, settings.LimitInGB})
	}
	if settings.LimitOutGB > 0 && monthOut >= mail.GBToBytes(settings.LimitOutGB) {
		out.LimitReached = append(out.LimitReached, TrafficThreshold{"traffic_limit_out", "出站限额", monthOut, settings.LimitOutGB})
	}
	if settings.LimitTotalGB > 0 && settings.LimitInGB > 0 && settings.LimitOutGB > 0 && total >= mail.GBToBytes(settings.LimitTotalGB) {
		out.LimitReached = append(out.LimitReached, TrafficThreshold{"traffic_limit_total", "总量限额", total, settings.LimitTotalGB})
	}
	return out
}

func CertHasIssue(c model.CertStatus, threshold int) bool {
	if !c.Present || !c.OK {
		return true
	}
	if c.DaysLeft != nil && *c.DaysLeft <= threshold {
		return true
	}
	if !c.TLSOK {
		return true
	}
	if c.TLSDaysLeft != nil && *c.TLSDaysLeft <= threshold {
		return true
	}
	if c.TLSHasLocalCert && !c.TLSMatchLocal {
		return true
	}
	return false
}

func CertAlertMessage(c model.CertStatus, threshold int) string {
	if !c.Present {
		return fmt.Sprintf("域名 %s 本地证书文件不存在（cert.pem not found）。", c.Domain)
	}
	if !c.TLSOK {
		return fmt.Sprintf("域名 %s 公网 TLS 检测失败，暂无法确认远端证书状态：%s。", c.Domain, c.TLSError)
	}
	if c.TLSHasLocalCert && !c.TLSMatchLocal {
		return fmt.Sprintf("域名 %s 公网握手证书与本地证书不一致，请检查反向代理或证书部署。", c.Domain)
	}
	if c.TLSDaysLeft != nil && *c.TLSDaysLeft <= threshold {
		return fmt.Sprintf("域名 %s 公网握手证书将在 %d 天后到期（到期时间：%s）。", c.Domain, *c.TLSDaysLeft, c.TLSExpiresAt)
	}
	if c.DaysLeft != nil {
		return fmt.Sprintf("域名 %s 本地证书将在 %d 天后到期（到期时间：%s）。", c.Domain, *c.DaysLeft, c.ExpiresAt)
	}
	return fmt.Sprintf("域名 %s 证书状态异常。", c.Domain)
}
