// SPDX-License-Identifier: Apache-2.0

package labcheck

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/config"
	"github.com/sagar2395/snowopslabs/internal/workload"
)

// A lab hostname must reach the ingress on loopback without an /etc/hosts
// entry and without a proxy, on whatever port the runtime bound.
func TestHTTPClientReachesLabHostsDirectly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "host="+r.Host)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	// A proxy that would swallow the request if it were used.
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("http_proxy", "http://127.0.0.1:1")

	cfg := &config.Config{Profile: "k3d", DomainSuffix: "lab.test", HTTPPort: port}
	resp, err := HTTPClient(cfg).Get(cfg.IngressURL("grafana") + "/api/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if want := "host=grafana.lab.test:" + port; string(body) != want {
		t.Errorf("body = %q, want %q (Host header must stay the lab name)", body, want)
	}
}

func TestHTTPClientLeavesOtherHostsAlone(t *testing.T) {
	cfg := &config.Config{Profile: "k3d", DomainSuffix: "lab.test"}
	tr := HTTPClient(cfg).Transport.(*http.Transport)
	t.Setenv("HTTPS_PROXY", "http://proxy.example:3128")
	req, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	u, err := tr.Proxy(req)
	if err != nil || u == nil || u.Host != "proxy.example:3128" {
		t.Errorf("non-lab hosts keep the environment's proxy, got %v, %v", u, err)
	}
}

func TestInclusterUsesAPlainClient(t *testing.T) {
	cfg := &config.Config{Profile: "incluster", DomainSuffix: "lab.example.com"}
	if HTTPClient(cfg).Transport != nil {
		t.Error("incluster hostnames resolve through real DNS; no dial override")
	}
}

func TestScriptEnvCarriesTheIngressPort(t *testing.T) {
	cfg := &config.Config{Profile: "k3d", DomainSuffix: "k3d.local", HTTPPort: "8080"}
	env := strings.Join(ScriptEnv(cfg, workload.Workload{Name: "go-api"}), "\n")
	for _, want := range []string{
		"INGRESS_URL_SUFFIX=k3d.local:8080",
		"GRAFANA_URL=http://grafana.k3d.local:8080",
		"WORKLOAD_NAME=go-api",
	} {
		if !strings.Contains(env, want) {
			t.Errorf("env missing %q:\n%s", want, env)
		}
	}
}

func TestURLsFollowTheIngressUnlessOverridden(t *testing.T) {
	cfg := &config.Config{Profile: "k3d", DomainSuffix: "k3d.local", HTTPPort: "8080"}
	t.Setenv("PROMETHEUS_URL", "")
	t.Setenv("ALERTMANAGER_URL", "")
	if got := PrometheusURL(cfg); got != "http://prometheus.k3d.local:8080" {
		t.Errorf("PrometheusURL = %q", got)
	}
	if got := AlertmanagerURL(cfg); got != "http://alertmanager.k3d.local:8080" {
		t.Errorf("AlertmanagerURL = %q", got)
	}
	t.Setenv("PROMETHEUS_URL", "http://localhost:9090")
	t.Setenv("ALERTMANAGER_URL", "http://localhost:9093")
	if got := PrometheusURL(cfg); got != "http://localhost:9090" {
		t.Errorf("PROMETHEUS_URL must win, got %q", got)
	}
	if got := AlertmanagerURL(cfg); got != "http://localhost:9093" {
		t.Errorf("ALERTMANAGER_URL must win, got %q", got)
	}
}

func TestNewRunnerIsWiredToTheLab(t *testing.T) {
	t.Setenv("PROMETHEUS_URL", "")
	cfg := &config.Config{Profile: "k3d", DomainSuffix: "k3d.local", HTTPPort: "80", ProjectRoot: "/lab"}
	r := NewRunner(cfg, workload.Workload{Name: "go-api", Namespace: "go-api"}, "/lab/scenarios/x")
	if r.PrometheusURL != "http://prometheus.k3d.local" {
		t.Errorf("PrometheusURL = %q", r.PrometheusURL)
	}
	if r.ScriptDir != "/lab/scenarios/x" {
		t.Errorf("ScriptDir = %q", r.ScriptDir)
	}
	if r.HTTPClient == nil || r.HTTPClient.Transport == nil {
		t.Error("a local lab needs the dialling client")
	}
	if !strings.Contains(strings.Join(r.Env, " "), "WORKLOAD_NAMESPACE=go-api") {
		t.Errorf("env = %v", r.Env)
	}
}
