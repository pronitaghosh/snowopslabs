// SPDX-License-Identifier: Apache-2.0

package k8s

import "testing"

func TestAllContainersReady(t *testing.T) {
	tests := []struct {
		name  string
		ready string
		want  bool
	}{
		{"single container ready", "1/1", true},
		{"sidecar not ready", "1/2", false},
		{"both ready", "2/2", true},
		{"nothing ready", "0/1", false},
		{"no containers reported", "0/0", false},
		{"malformed", "ready", false},
		{"empty", "", false},
		{"non-numeric", "a/b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := allContainersReady(tt.ready); got != tt.want {
				t.Errorf("allContainersReady(%q) = %v, want %v", tt.ready, got, tt.want)
			}
		})
	}
}

func TestPodHealth(t *testing.T) {
	tests := []struct {
		name      string
		pods      []PodInfo
		wantReady int
		wantTotal int
	}{
		{"none", nil, 0, 0},
		{"all running and ready", []PodInfo{{Status: "Running", Ready: "1/1"}, {Status: "Running", Ready: "2/2"}}, 2, 2},
		{"a pod not ready yet", []PodInfo{{Status: "Running", Ready: "1/1"}, {Status: "Running", Ready: "0/1"}}, 1, 2},
		{"a pending pod", []PodInfo{{Status: "Pending", Ready: "0/1"}}, 0, 1},
		{"a finished job counts as ready", []PodInfo{{Status: "Succeeded", Ready: "0/1"}}, 1, 1},
		{"an evicted pod beside its replacement is left out", []PodInfo{{Status: "Failed", Ready: "0/1"}, {Status: "Running", Ready: "1/1"}}, 1, 1},
		{"only failed pods", []PodInfo{{Status: "Failed", Ready: "0/1"}}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, total := podHealth(tt.pods)
			if ready != tt.wantReady || total != tt.wantTotal {
				t.Errorf("podHealth = %d/%d, want %d/%d", ready, total, tt.wantReady, tt.wantTotal)
			}
		})
	}
}
