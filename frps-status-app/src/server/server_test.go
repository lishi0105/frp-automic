package server

import (
	"reflect"
	"testing"

	"frps-status-app.local/status/src/model"
)

func TestInferProxyCertificateDomains(t *testing.T) {
	proxies := []model.ProxyTraffic{
		{Name: "emby", Type: "tcp"},
		{Name: "gitlab_web", Type: "tcp"},
		{Name: "speedtest", Type: "tcp", Domains: []string{"direct.example.com"}},
		{Name: "gitlab_ssh", Type: "tcp"},
	}
	certs := []model.CertStatus{
		{Domain: "emby.example.com"},
		{Domain: "gitlab-web.example.com"},
		{Domain: "status.example.com"},
		{Domain: "speedtest.example.com"},
	}

	inferProxyCertificateDomains(proxies, certs)

	cases := map[string][]string{
		"emby":       {"emby.example.com"},
		"gitlab_web": {"gitlab-web.example.com"},
		"speedtest":  {"direct.example.com", "speedtest.example.com"},
		"gitlab_ssh": nil,
	}
	for _, proxy := range proxies {
		if !reflect.DeepEqual(proxy.Domains, cases[proxy.Name]) {
			t.Fatalf("%s domains = %#v, want %#v", proxy.Name, proxy.Domains, cases[proxy.Name])
		}
	}
}

func TestAliasMatchKeys(t *testing.T) {
	got := aliasMatchKeys("GitLab_Web")
	want := []string{"gitlab_web", "gitlab-web"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("aliasMatchKeys() = %#v, want %#v", got, want)
	}
}
