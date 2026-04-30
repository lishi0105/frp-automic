package frps

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"frps-status-app.local/status/src/logger"
	"frps-status-app.local/status/src/model"
)

func CheckTCP(host string, port int) model.TCPCheck {
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		logger.Warn("TCP 连通性检测失败 主机=%s 端口=%d 错误=%v", host, port, err)
		return model.TCPCheck{OK: false, Error: err.Error()}
	}
	_ = conn.Close()
	ms := time.Since(start).Milliseconds()
	return model.TCPCheck{OK: true, LatencyMS: &ms}
}

func Certificates(certDir string, domains []string) []model.CertStatus {
	out := make([]model.CertStatus, 0, len(domains))
	for _, domain := range domains {
		raw, err := os.ReadFile(filepath.Join(certDir, domain, "cert.pem"))
		if err != nil {
			logger.Warn("证书文件不存在 域名=%s 路径=%s 错误=%v", domain, filepath.Join(certDir, domain, "cert.pem"), err)
			out = append(out, model.CertStatus{Domain: domain, Present: false, OK: false, Error: "cert.pem not found"})
			continue
		}
		block, _ := pem.Decode(raw)
		if block == nil {
			logger.Warn("证书 PEM 解码失败 域名=%s", domain)
			out = append(out, model.CertStatus{Domain: domain, Present: true, OK: false, Error: "parse certificate failed"})
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			logger.Warn("证书解析失败 域名=%s 错误=%v", domain, err)
			out = append(out, model.CertStatus{Domain: domain, Present: true, OK: false, Error: "parse certificate failed"})
			continue
		}
		days := int(time.Until(cert.NotAfter).Hours() / 24)
		out = append(out, model.CertStatus{Domain: domain, Present: true, OK: days >= 15, ExpiresAt: cert.NotAfter.UTC().Format(time.RFC3339), DaysLeft: &days})
	}
	return out
}

type Client struct {
	host          string
	dashboardPort int
	user          string
	pass          string
	httpClient    *http.Client
}

func NewClient(host string, dashboardPort int, user, pass string, httpClient *http.Client) *Client {
	return &Client{host: host, dashboardPort: dashboardPort, user: user, pass: pass, httpClient: httpClient}
}

func (c *Client) FetchProxies(ctx context.Context) ([]model.ProxyTraffic, error) {
	var all []model.ProxyTraffic
	var lastErr error
	for _, typ := range []string{"tcp", "http", "https", "tcpmux", "stcp", "xtcp", "udp"} {
		items, err := c.fetchProxyType(ctx, typ)
		if err != nil {
			logger.Warn("获取代理类型数据失败 类型=%s 错误=%v", typ, err)
			lastErr = err
			continue
		}
		logger.Info("获取代理类型成功 类型=%s 数量=%d", typ, len(items))
		all = append(all, items...)
	}
	if len(all) == 0 && lastErr != nil {
		logger.Error("获取代理列表失败，无有效数据 最后错误=%v", lastErr)
		return all, lastErr
	}
	logger.Info("获取代理列表完成 数量=%d", len(all))
	return all, nil
}

func (c *Client) fetchProxyType(ctx context.Context, typ string) ([]model.ProxyTraffic, error) {
	url := fmt.Sprintf("http://%s/api/proxy/%s", net.JoinHostPort(c.host, strconv.Itoa(c.dashboardPort)), typ)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		logger.Error("创建 FRPS 请求失败 地址=%s 错误=%v", url, err)
		return nil, err
	}
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		logger.Warn("FRPS 面板请求失败 地址=%s 错误=%v", url, err)
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		logger.Info("FRPS 代理接口不存在 类型=%s 地址=%s", typ, url)
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("%s returned %s", url, resp.Status)
		logger.Warn("FRPS 面板返回非成功状态 地址=%s 状态=%s", url, resp.Status)
		return nil, err
	}
	var raw any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		logger.Error("解析 FRPS 面板响应失败 地址=%s 错误=%v", url, err)
		return nil, err
	}
	var out []model.ProxyTraffic
	for _, obj := range extractObjects(raw) {
		// Merge nested info objects (proxyInfo, conf, config) into the top-level map
		// so fields like customDomains are accessible regardless of FRPS version.
		for _, key := range []string{"proxyInfo", "conf", "config", "info"} {
			if nested, ok := obj[key].(map[string]any); ok {
				for k, v := range nested {
					if _, exists := obj[k]; !exists {
						obj[k] = v
					}
				}
			}
		}
		name := stringValue(obj, "name", "proxyName")
		if name == "" {
			continue
		}
		out = append(out, model.ProxyTraffic{
			Name:       name,
			Type:       typ,
			Domains:    domainValues(obj),
			Online:     boolValue(obj, "online", "status"),
			CurConns:   int64(uintValue(obj, "curConns", "cur_conns")),
			CurrentIn:  uintValue(obj, "totalTrafficIn", "trafficIn", "todayTrafficIn", "in"),
			CurrentOut: uintValue(obj, "totalTrafficOut", "trafficOut", "todayTrafficOut", "out"),
		})
	}
	return out, nil
}

func domainValues(m map[string]any) []string {
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		seen[s] = struct{}{}
	}
	collectFrom := func(src map[string]any) {
		// Handle both snake_case (older FRPS) and camelCase (newer FRPS) domain fields.
		for _, key := range []string{"custom_domains", "customDomains", "domains"} {
			if arr, ok := src[key].([]any); ok {
				for _, v := range arr {
					if s, ok := v.(string); ok {
						add(s)
					}
				}
			}
		}
		for _, key := range []string{"subdomain", "domain"} {
			if s, ok := src[key].(string); ok {
				add(s)
			}
		}
	}
	collectFrom(m)
	// Also scan nested info objects in case they were not merged upstream.
	for _, key := range []string{"proxyInfo", "conf", "config", "info"} {
		if nested, ok := m[key].(map[string]any); ok {
			collectFrom(nested)
		}
	}
	out := make([]string, 0, len(seen))
	for d := range seen {
		out = append(out, d)
	}
	return out
}

func extractObjects(v any) []map[string]any {
	switch x := v.(type) {
	case []any:
		return mapsFromArray(x)
	case map[string]any:
		for _, key := range []string{"proxies", "proxyInfos", "tcp", "http", "https"} {
			if arr, ok := x[key].([]any); ok {
				return mapsFromArray(arr)
			}
		}
		if nested, ok := x["data"].(map[string]any); ok {
			return extractObjects(nested)
		}
	}
	return nil
}

func mapsFromArray(arr []any) []map[string]any {
	var out []map[string]any
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func stringValue(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s, ok := m[key].(string); ok {
			return s
		}
	}
	return ""
}

func boolValue(m map[string]any, keys ...string) bool {
	for _, key := range keys {
		switch v := m[key].(type) {
		case bool:
			return v
		case string:
			return strings.EqualFold(v, "online") || strings.EqualFold(v, "true") || strings.EqualFold(v, "running")
		}
	}
	return false
}

func uintValue(m map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		switch v := m[key].(type) {
		case float64:
			if v > 0 {
				return uint64(v)
			}
		case string:
			n, _ := strconv.ParseUint(v, 10, 64)
			return n
		}
	}
	return 0
}
