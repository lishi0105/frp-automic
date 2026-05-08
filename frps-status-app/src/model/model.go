package model

type TCPCheck struct {
	OK        bool   `json:"ok"`
	LatencyMS *int64 `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type CertStatus struct {
	Domain          string `json:"domain"`
	RelatedProxy    string `json:"relatedProxy,omitempty"`
	Present         bool   `json:"present"`
	OK              bool   `json:"ok"`
	ExpiresAt       string `json:"expires_at,omitempty"`
	DaysLeft        *int   `json:"days_left,omitempty"`
	Error           string `json:"error,omitempty"`
	TLSOK           bool   `json:"tls_ok"`
	TLSLatencyMS    *int64 `json:"tls_latency_ms,omitempty"`
	TLSExpiresAt    string `json:"tls_expires_at,omitempty"`
	TLSDaysLeft     *int   `json:"tls_days_left,omitempty"`
	TLSError        string `json:"tls_error,omitempty"`
	TLSMatchLocal   bool   `json:"tls_match_local"`
	TLSHasLocalCert bool   `json:"tls_has_local_cert"`
}

type ProxyHealth struct {
	ConsecutiveOffline int    `json:"consecutive_offline"`
	OnlineChecks       int64  `json:"online_checks"`
	TotalChecks        int64  `json:"total_checks"`
	OnlineRate         int    `json:"online_rate"`
	FlapCount24h       int    `json:"flap_count_24h"`
	LastChangeAt       string `json:"last_change_at,omitempty"`
	LastOfflineAt      string `json:"last_offline_at,omitempty"`
	LastRecoveryAt     string `json:"last_recovery_at,omitempty"`
	OfflineSeconds     int64  `json:"offline_seconds,omitempty"`
}

type ProxyTraffic struct {
	Name       string      `json:"name"`
	Type       string      `json:"type"`
	Domains    []string    `json:"domains,omitempty"`
	Online     bool        `json:"online"`
	CurConns   int64       `json:"cur_conns"`
	CurrentIn  uint64      `json:"current_in"`
	CurrentOut uint64      `json:"current_out"`
	MonthIn    uint64      `json:"month_in"`
	MonthOut   uint64      `json:"month_out"`
	Health     ProxyHealth `json:"health"`
}

type PublicSettings struct {
	ThresholdInGB        float64 `json:"threshold_in_gb"`
	ThresholdOutGB       float64 `json:"threshold_out_gb"`
	ThresholdTotalGB     float64 `json:"threshold_total_gb"`
	LimitInGB            float64 `json:"limit_in_gb"`
	LimitOutGB           float64 `json:"limit_out_gb"`
	LimitTotalGB         float64 `json:"limit_total_gb"`
	HistoryRetentionDays int     `json:"history_retention_days"`
	SMTPHost             string  `json:"smtp_host"`
	SMTPPort             int     `json:"smtp_port"`
	SMTPUser             string  `json:"smtp_user"`
	SMTPFrom             string  `json:"smtp_from"`
	SMTPTo               string  `json:"smtp_to"`
	SMTPEnabled          bool    `json:"smtp_enabled"`
	SMTPAuthCode         string  `json:"smtp_auth_code"`
	AlertProxyOffline    bool    `json:"alert_proxy_offline"`
	AlertCertExpiry      bool    `json:"alert_cert_expiry"`
	AlertCertDays        int     `json:"alert_cert_days"`
}

type Snapshot struct {
	GeneratedAt  string            `json:"generated_at"`
	FRPS         map[string]any    `json:"frps"`
	Certificates []CertStatus      `json:"certificates"`
	Proxies      []ProxyTraffic    `json:"proxies"`
	MonthTotals  map[string]uint64 `json:"month_totals"`
	Dashboard    DashboardSummary  `json:"dashboard"`
	Settings     PublicSettings    `json:"settings"`
}

type DashboardTopProxy struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	MonthIn  uint64 `json:"month_in"`
	MonthOut uint64 `json:"month_out"`
	Total    uint64 `json:"total"`
}

type DashboardCertSummary struct {
	Total       int    `json:"total"`
	OK          int    `json:"ok"`
	Warn        int    `json:"warn"`
	Fail        int    `json:"fail"`
	MinDomain   string `json:"min_domain,omitempty"`
	MinDaysLeft *int   `json:"min_days_left,omitempty"`
}

type DashboardSummary struct {
	TopProxies  []DashboardTopProxy  `json:"top_proxies"`
	Certificate DashboardCertSummary `json:"certificate"`
}

type PagedMeta struct {
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

type ProxyListResponse struct {
	Items []ProxyTraffic `json:"items"`
	Meta  PagedMeta      `json:"meta"`
}

type CertificateListResponse struct {
	Items []CertStatus `json:"items"`
	Meta  PagedMeta    `json:"meta"`
}
