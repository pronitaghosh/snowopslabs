// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/config"
	"github.com/sagar2395/snowopslabs/internal/platform"
)

func TestResolveProvider(t *testing.T) {
	root := t.TempDir()
	writeProvider(t, root, "mesh/istio")
	writeProvider(t, root, "mesh/linkerd")
	writeProvider(t, root, "cost/opencost")
	oldReg, oldCfg := reg, cfg
	t.Cleanup(func() { reg, cfg = oldReg, oldCfg })
	reg = platform.NewRegistry(root)

	tests := []struct {
		name     string
		cfg      config.Config
		category string
		expected string
		wantErr  []string
	}{
		{name: "the configured provider", cfg: config.Config{MeshProvider: "linkerd"}, category: "mesh", expected: "linkerd"},
		{name: "the only provider", category: "cost", expected: "opencost"},
		{
			name:     "several and none configured names both ways to choose",
			category: "mesh",
			wantErr:  []string{"istio, linkerd", "labctl platform up mesh/istio", "MESH_PROVIDER=istio in .env"},
		},
		{name: "unknown category", category: "nope", wantErr: []string{`unknown platform category "nope"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.cfg
			cfg = &c
			got, err := resolveProvider(tt.category)
			if len(tt.wantErr) == 0 {
				if err != nil || got != tt.expected {
					t.Fatalf("resolveProvider(%q) = %q, %v; want %q", tt.category, got, err, tt.expected)
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
