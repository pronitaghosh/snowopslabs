// SPDX-License-Identifier: Apache-2.0

package cli

import "testing"

func TestNodeContainerLabel(t *testing.T) {
	tests := []struct {
		name     string
		profile  string
		expected string
		ok       bool
	}{
		{name: "k3d", profile: "k3d", expected: "k3d.cluster=lab", ok: true},
		{name: "kind", profile: "kind", expected: "io.x-k8s.kind.cluster=lab", ok: true},
		{name: "incluster has no node containers", profile: "incluster", expected: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := nodeContainerLabel(tt.profile, "lab")
			if got != tt.expected || ok != tt.ok {
				t.Errorf("nodeContainerLabel(%q) = %q, %v; want %q, %v", tt.profile, got, ok, tt.expected, tt.ok)
			}
		})
	}
}
