// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/capacity"
)

const demandScenario = `name: kafka-lab
displayName: Kafka Lab
description: x
category: testing
requirements:
  memory: 300Mi
  agents: 2
  cpus: 4
prerequisites:
  apps:
    - "{{.WorkloadName}}"
  platform:
    - ingress
    - monitoring/metrics
    - data/kafka
components:
  - name: loki
    type: helm
    chart: grafana/loki
    namespace: "{{.MonitoringNamespace}}"
  - name: seed
    type: script
    script: seed.sh
`

func TestEngine_Demand(t *testing.T) {
	root := t.TempDir()
	writeScenario(t, root, "kafka-lab", demandScenario)
	eng := NewEngine(root, "k3d.local", "k3d", "observability")
	s, err := eng.Get("kafka-lab")
	if err != nil {
		t.Fatal(err)
	}

	got, err := eng.Demand(s)
	if err != nil {
		t.Fatal(err)
	}
	expectedItems := []capacity.Item{
		{Kind: capacity.ItemPlatform, Name: "data/kafka"},
		{Kind: capacity.ItemRelease, Name: "observability/loki"},
		{Kind: capacity.ItemApp, Name: "go-api"},
	}
	if !slices.Equal(got.Items, expectedItems) {
		t.Errorf("Items = %v, want %v (baseline platform and non-helm components excluded)", got.Items, expectedItems)
	}
	if got.OwnMiB != 300 || got.Agents != 2 || got.CPUs != 4 || got.App != "go-api" {
		t.Errorf("Demand = %+v", got)
	}
}

func TestEngine_Demand_UnreadableMemoryKeepsSharedItems(t *testing.T) {
	eng := NewEngine(t.TempDir(), "k3d.local", "k3d", "monitoring")
	s := &Scenario{
		Name:          "broken",
		Requirements:  Requirements{Memory: "lots"},
		Prerequisites: Prerequisites{Platform: []string{"data/kafka"}},
	}
	got, err := eng.Demand(s)
	if err == nil {
		t.Fatal("expected an error for an unreadable memory requirement")
	}
	if len(got.Items) != 1 {
		t.Errorf("shared items must survive the error, got %v", got.Items)
	}
}

func TestEngine_ActiveDemands(t *testing.T) {
	root := t.TempDir()
	writeScenario(t, root, "kafka-lab", demandScenario)
	writeScenario(t, root, "plain", "name: plain\ndisplayName: Plain\ndescription: x\ncategory: testing\n")
	writeScenario(t, root, "idle", "name: idle\ndisplayName: Idle\ndescription: x\ncategory: testing\n")
	eng := NewEngine(root, "k3d.local", "k3d", "monitoring")
	for _, name := range []string{"kafka-lab", "plain"} {
		if err := eng.markActive(name, nil); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		except   string
		expected []string
	}{
		{name: "every active scenario", except: "", expected: []string{"kafka-lab", "plain"}},
		{name: "without the one being started", except: "plain", expected: []string{"kafka-lab"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, d := range eng.ActiveDemands(tt.except) {
				got = append(got, d.Name)
			}
			slices.Sort(got)
			if !slices.Equal(got, tt.expected) {
				t.Errorf("ActiveDemands(%q) = %v, want %v", tt.except, got, tt.expected)
			}
		})
	}
}

func TestEngine_Up_AsksAdmit(t *testing.T) {
	yaml := `name: gated
displayName: Gated
description: x
category: testing
requirements:
  memory: 1Gi
components:
  - name: step
    type: script
    script: step.sh
`
	tests := []struct {
		name        string
		admitErr    error
		wantErr     string
		wantActive  bool
		wantInstall bool
	}{
		{name: "admitted", wantActive: true, wantInstall: true},
		{name: "blocked", admitErr: errors.New("not enough memory"), wantErr: "gated cannot start: not enough memory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeScenario(t, root, "gated", yaml)
			if err := os.WriteFile(filepath.Join(root, "scenarios", "gated", "step.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil { //nolint:gosec // test script must be executable
				t.Fatal(err)
			}
			eng := NewEngine(root, "k3d.local", "k3d", "monitoring")
			eng.SetOutput(io.Discard)
			var asked capacity.Demand
			eng.Admit = func(_ context.Context, _ io.Writer, next capacity.Demand) error {
				asked = next
				return tt.admitErr
			}

			rec := &recordingExec{}
			err := eng.Up("gated", rec, false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Up error = %v, want %q", err, tt.wantErr)
				}
			} else if err != nil {
				t.Fatalf("Up: %v", err)
			}
			if asked.Name != "gated" || asked.OwnMiB != 1024 {
				t.Errorf("Admit was asked about %+v", asked)
			}
			if got := eng.isActive("gated"); got != tt.wantActive {
				t.Errorf("active = %v, want %v", got, tt.wantActive)
			}
			if got := len(rec.calls) > 0; got != tt.wantInstall {
				t.Errorf("components installed = %v, want %v", got, tt.wantInstall)
			}
		})
	}
}
