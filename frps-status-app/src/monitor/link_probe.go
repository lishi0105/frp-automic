package monitor

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// LinkHealth 表示检测机本机 DNS 与外网连通性（告警父级优先级 1）。
type LinkHealth struct {
	DNSOK         bool
	DNSError      string
	OutboundOK    bool
	OutboundError string
}

// OK 为 true 表示 DNS 与 HTTPS 外网探测均成功。
func (h LinkHealth) OK() bool {
	return h.DNSOK && h.OutboundOK
}

const defaultDNSProbeHost = "example.com"
const defaultOutboundURL = "https://example.com/"

// ProbeLinkHealth 探测 DNS 解析与外网 HTTPS 可达性；任一步失败则检测链路视为不可用。
func ProbeLinkHealth(ctx context.Context) LinkHealth {
	var h LinkHealth
	h.DNSOK = true
	h.OutboundOK = true
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := net.DefaultResolver.LookupHost(ctx, defaultDNSProbeHost)
	if err != nil {
		h.DNSOK = false
		h.DNSError = err.Error()
		h.OutboundOK = false
		h.OutboundError = "因 DNS 失败未进行外网 HTTPS 探测"
		return h
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, defaultOutboundURL, nil)
	if err != nil {
		h.OutboundOK = false
		h.OutboundError = err.Error()
		return h
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		h.OutboundOK = false
		h.OutboundError = err.Error()
		return h
	}
	defer resp.Body.Close()
	_, _ = io.CopyN(io.Discard, resp.Body, 2048)
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		h.OutboundOK = false
		h.OutboundError = fmt.Sprintf("HTTP %s", resp.Status)
		return h
	}
	return h
}

// LinkHealthSummary 用于父级告警正文。
func LinkHealthSummary(h LinkHealth) string {
	var b strings.Builder
	b.WriteString("检测链路探测（本机）：\n")
	if h.DNSOK {
		b.WriteString(fmt.Sprintf("- DNS（%s）：正常\n", defaultDNSProbeHost))
	} else {
		b.WriteString(fmt.Sprintf("- DNS（%s）：失败 — %s\n", defaultDNSProbeHost, h.DNSError))
	}
	if h.OutboundOK {
		b.WriteString(fmt.Sprintf("- 外网 HTTPS（%s）：正常\n", defaultOutboundURL))
	} else {
		b.WriteString(fmt.Sprintf("- 外网 HTTPS（%s）：失败 — %s\n", defaultOutboundURL, h.OutboundError))
	}
	b.WriteString("\n代理与证书在线检测结论已抑制为「未知」，子告警邮件已暂停；待检测链路恢复后重新检测。")
	return b.String()
}
