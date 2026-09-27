// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// labRoot makes a minimal project with a k3d profile and the given .env.
func labRoot(t *testing.T, dotenv string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"scenarios", "runtimes/k3d"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(dotenv), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestRecordedPortsFollowTheCluster(t *testing.T) {
	for _, k := range []string{"HTTP_PORT", "HTTPS_PORT", "CLUSTER_NAME", "PROFILE", "DOMAIN_SUFFIX"} {
		t.Setenv(k, "")
	}
	home := t.TempDir()
	t.Setenv("SNOWOPS_HOME", home)
	root := labRoot(t, "HTTP_PORT=80\nCLUSTER_NAME=lab\n")

	t.Run("no record keeps .env", func(t *testing.T) {
		cfg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.IngressURL("grafana"); got != "http://grafana.k3d.local" {
			t.Errorf("IngressURL = %q", got)
		}
	})

	if err := os.MkdirAll(filepath.Join(home, "clusters"), 0o755); err != nil {
		t.Fatal(err)
	}
	rec := "HTTP_PORT=8080\nHTTPS_PORT=8443\n"
	if err := os.WriteFile(filepath.Join(home, "clusters", "lab.env"), []byte(rec), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("the runtime's record beats .env", func(t *testing.T) {
		cfg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.HTTPPort != "8080" || cfg.HTTPSPort != "8443" {
			t.Errorf("ports = %s/%s, want 8080/8443", cfg.HTTPPort, cfg.HTTPSPort)
		}
		if got := cfg.IngressURLSuffix(); got != "k3d.local:8080" {
			t.Errorf("IngressURLSuffix = %q", got)
		}
		if got := cfg.IngressURL("grafana"); got != "http://grafana.k3d.local:8080" {
			t.Errorf("IngressURL = %q", got)
		}
		if cfg.ScriptEnv["HTTP_PORT"] != "8080" {
			t.Errorf("scripts must see the recorded port, got %q", cfg.ScriptEnv["HTTP_PORT"])
		}
	})

	t.Run("a real environment variable still wins", func(t *testing.T) {
		t.Setenv("HTTP_PORT", "9090")
		cfg, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.HTTPPort != "9090" {
			t.Errorf("HTTPPort = %q, want 9090", cfg.HTTPPort)
		}
	})
}

func TestLocalIngress(t *testing.T) {
	for profile, want := range map[string]bool{"k3d": true, "kind": true, "incluster": false} {
		if got := (&Config{Profile: profile}).LocalIngress(); got != want {
			t.Errorf("%s: LocalIngress = %v, want %v", profile, got, want)
		}
	}
}
