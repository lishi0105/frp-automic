package model

type TCPCheck struct {
	OK        bool   `json:"ok"`
	LatencyMS *int64 `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type CertStatus struct {
	Domain    string `json:"domain"`
	Present   bool   `json:"present"`
	OK        bool   `json:"ok"`
	ExpiresAt string `json:"expires_at,omitempty"`
	DaysLeft  *int   `json:"days_left,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ProxyTraffic struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Online     bool   `json:"online"`
	CurConns   int64  `json:"cur_conns"`
	CurrentIn  uint64 `json:"current_in"`
	CurrentOut uint64 `json:"current_out"`
	MonthIn    uint64 `json:"month_in"`
	MonthOut   uint64 `json:"month_out"`
}

type PublicSettings struct {
	AlertInGB       float64 `json:"alert_in_gb"`
	AlertOutGB      float64 `json:"alert_out_gb"`
	SMTPHost        string  `json:"smtp_host"`
	SMTPPort        int     `json:"smtp_port"`
	SMTPUser        string  `json:"smtp_user"`
	SMTPFrom        string  `json:"smtp_from"`
	SMTPTo          string  `json:"smtp_to"`
	SMTPEnabled     bool    `json:"smtp_enabled"`
	SMTPPasswordSet bool    `json:"smtp_password_set"`
}

type Snapshot struct {
	GeneratedAt  string            `json:"generated_at"`
	FRPS         map[string]any    `json:"frps"`
	Certificates []CertStatus      `json:"certificates"`
	Proxies      []ProxyTraffic    `json:"proxies"`
	MonthTotals  map[string]uint64 `json:"month_totals"`
	Settings     PublicSettings    `json:"settings"`
}
