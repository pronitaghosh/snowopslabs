// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/platform"
)

// writeProvider creates platform/<relDir>/install.sh so the registry scan
// discovers a provider at that path.
func writeProvider(t *testing.T, root, relDir string) {
	t.Helper()
	dir := filepath.Join(root, "platform", relDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "install.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestPrereqProviders(t *testing.T) {
	root := t.TempDir()
	writeProvider(t, root, "cost/opencost")                 // category/provider
	writeProvider(t, root, "monitoring/metrics/prometheus") // sub-category with a provider
	writeProvider(t, root, "ingress/traefik")               // bare category, provider owns its ns
	writeProvider(t, root, "ingress/nginx")                 // second mutually-exclusive provider
	reg := platform.NewRegistry(root)

	tests := []struct {
		name     string
		prereq   string
		expected []string
	}{
		{name: "category/provider resolves to that provider", prereq: "cost/opencost", expected: []string{"opencost"}},
		{name: "sub-category resolves to its provider", prereq: "monitoring/metrics", expected: []string{"prometheus"}},
		{name: "bare category resolves to every provider", prereq: "ingress", expected: []string{"nginx", "traefik"}},
		{name: "unknown prereq resolves to nothing", prereq: "does/not/exist", expected: []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := []string{}
			for _, p := range prereqProviders(reg, tt.prereq) {
				got = append(got, p.Name)
			}
			sort.Strings(got)
			if !slices.Equal(got, tt.expected) {
				t.Errorf("prereqProviders(%q) = %v, want %v", tt.prereq, got, tt.expected)
			}
		})
	}
}

// stubKubectl puts a fake kubectl first on PATH that answers `get namespace`
// successfully only for the namespaces listed.
func stubKubectl(t *testing.T, namespaces ...string) {
	t.Helper()
	dir := t.TempDir()
	var script strings.Builder
	script.WriteString("#!/bin/sh\ncase \"$*\" in\n")
	for _, ns := range namespaces {
		script.WriteString("  \"get namespace " + ns + " --no-headers\") exit 0 ;;\n")
	}
	script.WriteString("esac\nexit 1\n")
	if err := os.WriteFile(filepath.Join(dir, "kubectl"), []byte(script.String()), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestEnsurePlatformPrereqs(t *testing.T) {
	root := t.TempDir()
	writeProvider(t, root, "cost/opencost")
	oldReg := reg
	t.Cleanup(func() { reg = oldReg })
	reg = platform.NewRegistry(root)

	tests := []struct {
		name      string
		installed []string
		prereqs   []string
		wantErr   []string
	}{
		{name: "installed", installed: []string{"opencost"}, prereqs: []string{"cost/opencost"}},
		{
			name:    "missing is an error with the install command",
			prereqs: []string{"cost/opencost"},
			wantErr: []string{"not installed: cost/opencost", "labctl platform up cost/opencost", "--deploy-prereqs"},
		},
		{name: "unknown to the registry is left to preflight", prereqs: []string{"does/not/exist"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubKubectl(t, tt.installed...)
			var out bytes.Buffer
			err := ensurePlatformPrereqs(t.Context(), &out, tt.prereqs, false)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			for _, want := range tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q, got %v", want, err)
				}
			}
		})
	}
}
