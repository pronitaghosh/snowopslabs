// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"testing"

	"github.com/sagar2395/snowopslabs/internal/config"
	"github.com/sagar2395/snowopslabs/internal/incident"
	"github.com/sagar2395/snowopslabs/internal/learn"
)

func TestExpandVars(t *testing.T) {
	t.Setenv("MY_HOST", "example.com")
	cases := []struct {
		in, suffix, want string
	}{
		// ${VAR:-default} form.
		{"http://go-api.${DOMAIN_SUFFIX:-k3d.local}/health", "prod.internal", "http://go-api.prod.internal/health"},
		// default applies when the suffix is empty.
		{"http://go-api.${DOMAIN_SUFFIX:-k3d.local}/health", "", "http://go-api.k3d.local/health"},
		// plain ${VAR} form.
		{"http://x.${DOMAIN_SUFFIX}/y", "k3d.local", "http://x.k3d.local/y"},
		// other env vars still resolve.
		{"http://${MY_HOST}/z", "k3d.local", "http://example.com/z"},
		// nothing to expand.
		{"http://static/health", "k3d.local", "http://static/health"},
	}
	for _, c := range cases {
		if got := expandVars(c.in, c.suffix); got != c.want {
			t.Errorf("expandVars(%q, %q) = %q, want %q", c.in, c.suffix, got, c.want)
		}
	}
}

func TestChecksCheckHTTP(t *testing.T) {
	// expectStatus (canonical) must be honoured, and the URL fully expanded so
	// url.Parse never sees a raw ${...}.
	c := learn.Check{
		Name:         "go-api-healthy",
		Type:         "http",
		URL:          "http://go-api.${DOMAIN_SUFFIX:-k3d.local}/health",
		ExpectStatus: 200,
	}
	got := checksCheck(c, "/tmp", func(s string) string { return expandVars(s, "k3d.local") })
	if got.URL != "http://go-api.k3d.local/health" {
		t.Errorf("URL = %q, want expanded", got.URL)
	}
	if got.ExpectStatus != 200 {
		t.Errorf("ExpectStatus = %d, want 200", got.ExpectStatus)
	}
}

func TestLearnResolver(t *testing.T) {
	oldCfg, oldInc := cfg, incEng
	t.Cleanup(func() { cfg, incEng = oldCfg, oldInc })
	cfg = &config.Config{DomainSuffix: "lab.localhost", HTTPPort: "8080"}
	incEng = incident.NewEngine(t.TempDir(), "lab.localhost")
	incEng.IngressURLSuffix = cfg.IngressURLSuffix()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "a template carries the port", input: "http://go-api.{{.IngressURLSuffix}}/health", expected: "http://go-api.lab.localhost:8080/health"},
		{name: "an environment variable still expands", input: "http://go-api.${DOMAIN_SUFFIX}/health", expected: "http://go-api.lab.localhost/health"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := learnResolver()(tt.input); got != tt.expected {
				t.Errorf("resolve(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
