// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// footprints is a small, round set of costs for the tests below.
var footprints = Footprints{
	Baseline: 2000,
	Agent:    500,
	Releases: map[string]int{"loki": 300, "tempo": 200},
	Platform: map[string]int{"data/kafka": 1200},
	Apps:     map[string]int{"go-api": 100},
	Unlisted: 50,
}

var (
	loki  = Item{Kind: ItemRelease, Name: "monitoring/loki"}
	tempo = Item{Kind: ItemRelease, Name: "monitoring/tempo"}
	kafka = Item{Kind: ItemPlatform, Name: "data/kafka"}
	goAPI = Item{Kind: ItemApp, Name: "go-api"}
)

// lab4GB is a colima VM sized at the floor: 85% of 3.8 GiB ≈ 3309 MiB usable.
func lab4GB() Lab {
	return Lab{
		Engine:       Resources{CPUs: 2, MemBytes: 4094 << 20},
		Host:         Host{Engine: EngineColima},
		Agents:       1,
		CanAddAgents: true,
	}
}

func TestUsableMiB(t *testing.T) {
	tests := []struct {
		name     string
		input    Resources
		expected int
	}{
		{name: "4 GiB", input: Resources{CPUs: 2, MemBytes: 4 << 30}, expected: 3481},
		{name: "none", input: Resources{}, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UsableMiB(tt.input); got != tt.expected {
				t.Errorf("UsableMiB = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestLoadFootprints(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	tests := []struct {
		name     string
		file     string
		expected Footprints
		wantErr  bool
	}{
		{name: "missing file is empty", file: filepath.Join(dir, "absent.yaml"), expected: Footprints{}},
		{
			name:     "reads every table",
			file:     write("ok.yaml", "baseline: 2100\nagent: 450\nreleases: {loki: 300}\nunlisted: 64\n"),
			expected: Footprints{Baseline: 2100, Agent: 450, Releases: map[string]int{"loki": 300}, Unlisted: 64},
		},
		{name: "malformed yaml", file: write("bad.yaml", "baseline: [\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadFootprints(tt.file)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadFootprints error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Baseline != tt.expected.Baseline || got.Agent != tt.expected.Agent ||
				got.Unlisted != tt.expected.Unlisted || got.Releases["loki"] != tt.expected.Releases["loki"] {
				t.Errorf("got %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestFootprints_Cost(t *testing.T) {
	tests := []struct {
		name     string
		item     Item
		expected int
	}{
		{name: "release by name, whatever the namespace", item: Item{Kind: ItemRelease, Name: "logging/loki"}, expected: 300},
		{name: "platform component", item: kafka, expected: 1200},
		{name: "app", item: goAPI, expected: 100},
		{name: "unlisted", item: Item{Kind: ItemApp, Name: "java-api"}, expected: 50},
		{name: "unknown kind costs nothing", item: Item{Name: "x"}, expected: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := footprints.cost(tt.item); got != tt.expected {
				t.Errorf("cost = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestVerdict_Allowed(t *testing.T) {
	tests := []struct {
		name     string
		verdict  Verdict
		expected bool
	}{
		{name: "no problems", verdict: Verdict{Warnings: []string{"slow"}}, expected: true},
		{name: "a problem", verdict: Verdict{Problems: []string{"too big"}}, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.verdict.Allowed(); got != tt.expected {
				t.Errorf("Allowed = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	obs := Demand{Name: "observability-sre", Items: []Item{loki, tempo, goAPI}, OwnMiB: 100, App: "go-api"}
	chaos := Demand{Name: "chaos-engineering", Items: []Item{loki, goAPI}, OwnMiB: 50}
	kafkaDemand := Demand{Name: "event-driven-arch", Items: []Item{kafka, goAPI}, OwnMiB: 200}

	tests := []struct {
		name          string
		lab           func() Lab
		active        []Demand
		next          Demand
		allowed       bool
		needMiB       int
		addAgents     int
		wantProblems  []string
		wantWarnings  []string
		avoidProblems []string
	}{
		{
			name:    "fits on its own",
			lab:     lab4GB,
			next:    obs,
			allowed: true,
			needMiB: 2000 + 300 + 200 + 100 + 100,
		},
		{
			name:    "a release shared with an active scenario is counted once",
			lab:     lab4GB,
			active:  []Demand{obs},
			next:    chaos,
			allowed: true,
			needMiB: 2000 + 300 + 200 + 100 + 100 + 50,
		},
		{
			name:    "too big even alone offers only the resize",
			lab:     lab4GB,
			active:  []Demand{obs},
			next:    kafkaDemand,
			allowed: false,
			needMiB: 2000 + 300 + 200 + 100 + 1200 + 100 + 200,
			wantProblems: []string{
				"Not enough memory for event-driven-arch",
				"about 4 GB and may use 3.4 GB (85% of Docker's 4 GB",
				"Give Docker 5 GB",
				"colima stop && colima start --cpu 2 --memory 5",
			},
			avoidProblems: []string{"scenario down"},
		},
		{
			name:    "names what to bring down when that makes room",
			lab:     lab4GB,
			active:  []Demand{obs, {Name: "cost-right-sizing", OwnMiB: 800}},
			next:    chaos,
			allowed: false,
			needMiB: 2000 + 300 + 200 + 100 + 100 + 800 + 50,
			wantProblems: []string{
				"Either free memory:\n  labctl scenario down cost-right-sizing",
				"or give Docker 5 GB",
			},
		},
		{
			name: "live usage beyond the footprints still blocks",
			lab: func() Lab {
				l := lab4GB()
				l.UsedMiB = 3200
				return l
			},
			next:         chaos,
			allowed:      false,
			needMiB:      3200 + 50 + 300 + 100,
			wantProblems: []string{"Not enough memory for chaos-engineering"},
		},
		{
			name: "live usage does not charge again for what is already running",
			lab: func() Lab {
				l := lab4GB()
				l.UsedMiB = 2900
				l.Running = map[Item]bool{loki: true, goAPI: true}
				return l
			},
			next:    chaos,
			allowed: true,
			needMiB: 2900 + 50,
		},
		{
			name:         "nothing can run beside an exclusive activation",
			lab:          lab4GB,
			active:       []Demand{{Name: "cluster-upgrade-drill", Exclusive: true}},
			next:         chaos,
			allowed:      false,
			wantProblems: []string{"cluster-upgrade-drill is active and must run on its own"},
		},
		{
			name:         "an exclusive activation waits for the others",
			lab:          lab4GB,
			active:       []Demand{chaos},
			next:         Demand{Name: "cluster-upgrade-drill", Exclusive: true},
			allowed:      false,
			wantProblems: []string{"must run on its own", "labctl scenario down chaos-engineering"},
		},
		{
			name:      "agents are added when the runtime can",
			lab:       lab4GB,
			next:      Demand{Name: "node-drain-drill", Agents: 2},
			allowed:   true,
			needMiB:   2000 + 500,
			addAgents: 1,
		},
		{
			name: "a runtime that cannot add agents is told to rebuild",
			lab: func() Lab {
				l := lab4GB()
				l.CanAddAgents = false
				return l
			},
			next:         Demand{Name: "node-drain-drill", Agents: 2},
			allowed:      false,
			wantProblems: []string{"set AGENTS=2", "labctl reset"},
		},
		{
			name:         "too few cpus only warns",
			lab:          lab4GB,
			next:         Demand{Name: "mesh-traffic-management", CPUs: 4},
			allowed:      true,
			wantWarnings: []string{"runs best with 4 CPUs", "colima stop && colima start --cpu 4"},
		},
		{
			name:         "sharing the bound app only warns",
			lab:          lab4GB,
			active:       []Demand{obs},
			next:         Demand{Name: "autoscaling-under-load", App: "go-api"},
			allowed:      true,
			wantWarnings: []string{"observability-sre also uses go-api", "--app"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Evaluate(footprints, tt.lab(), tt.active, tt.next)
			if v.Allowed() != tt.allowed {
				t.Fatalf("Allowed = %v, want %v; problems: %v", v.Allowed(), tt.allowed, v.Problems)
			}
			if tt.needMiB != 0 && v.NeedMiB != tt.needMiB {
				t.Errorf("NeedMiB = %d, want %d", v.NeedMiB, tt.needMiB)
			}
			if v.AddAgents != tt.addAgents {
				t.Errorf("AddAgents = %d, want %d", v.AddAgents, tt.addAgents)
			}
			problems, warnings := strings.Join(v.Problems, "\n"), strings.Join(v.Warnings, "\n")
			for _, want := range tt.wantProblems {
				if !strings.Contains(problems, want) {
					t.Errorf("problems should mention %q:\n%s", want, problems)
				}
			}
			for _, avoid := range tt.avoidProblems {
				if strings.Contains(problems, avoid) {
					t.Errorf("problems should not mention %q:\n%s", avoid, problems)
				}
			}
			for _, want := range tt.wantWarnings {
				if !strings.Contains(warnings, want) {
					t.Errorf("warnings should mention %q:\n%s", want, warnings)
				}
			}
		})
	}
}

func TestResizeGiB(t *testing.T) {
	tests := []struct {
		name     string
		engine   Resources
		needMiB  int
		expected float64
	}{
		{name: "covers the need after the vm's own share", engine: Resources{MemBytes: 4094 << 20}, needMiB: 4096, expected: 5},
		{name: "never the size the engine already has", engine: Resources{MemBytes: 4094 << 20}, needMiB: 3482, expected: 5},
		{name: "a large need", engine: Resources{MemBytes: 4094 << 20}, needMiB: 6000, expected: 8},
		{name: "never below the floor", engine: Resources{MemBytes: 1 << 30}, needMiB: 900, expected: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resizeGiB(tt.engine, tt.needMiB); got != tt.expected {
				t.Errorf("resizeGiB = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestMakeRoom(t *testing.T) {
	small := Demand{Name: "small", OwnMiB: 100}
	big := Demand{Name: "big", OwnMiB: 900}
	shared := Demand{Name: "shares-loki", Items: []Item{loki}}
	withKafka := Demand{Name: "kafka-lab", OwnMiB: 100, Items: []Item{{Kind: ItemPlatform, Name: "data"}}}
	fault := Demand{Name: "incident oom-kill", OwnMiB: 400}
	next := Demand{Name: "next", Items: []Item{loki}}

	tests := []struct {
		name      string
		active    []Demand
		installed []Item
		short     int
		expected  []string
	}{
		{name: "largest saving first", active: []Demand{small, big}, short: 500,
			expected: []string{"labctl scenario down big"}},
		{name: "several when one is not enough", active: []Demand{small, big}, short: 950,
			expected: []string{"labctl scenario down big", "labctl scenario down small"}},
		{name: "not enough even with everything down", active: []Demand{small}, short: 500, expected: nil},
		{name: "a release next also uses frees nothing", active: []Demand{shared}, short: 100, expected: nil},
		{name: "a component nothing uses can make room", active: []Demand{small}, installed: []Item{kafka}, short: 1000,
			expected: []string{"labctl platform down data/kafka"}},
		{name: "a scenario comes down with the components only it uses", active: []Demand{withKafka}, installed: []Item{kafka}, short: 1000,
			expected: []string{"labctl scenario down kafka-lab", "labctl platform down data/kafka"}},
		{name: "a scenario down alone does not free its components", active: []Demand{withKafka}, short: 1000, expected: nil},
		{name: "a fault is resolved, not brought down", active: []Demand{fault}, short: 300,
			expected: []string{"labctl incident resolve"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := makeRoom(footprints, Lab{Installed: tt.installed}, tt.active, next, tt.short)
			if !slices.Equal(got, tt.expected) {
				t.Errorf("makeRoom = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestItem_Covers(t *testing.T) {
	istio := Item{Kind: ItemPlatform, Name: "mesh/istio"}
	tests := []struct {
		name     string
		item     Item
		other    Item
		expected bool
	}{
		{name: "equal items", item: kafka, other: kafka, expected: true},
		{name: "a category covers its provider", item: Item{Kind: ItemPlatform, Name: "mesh"}, other: istio, expected: true},
		{name: "a provider does not cover its category", item: istio, other: Item{Kind: ItemPlatform, Name: "mesh"}, expected: false},
		{name: "another category", item: Item{Kind: ItemPlatform, Name: "data"}, other: istio, expected: false},
		{name: "a release is not a platform category", item: Item{Kind: ItemRelease, Name: "mesh"}, other: istio, expected: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.item.Covers(tt.other); got != tt.expected {
				t.Errorf("Covers = %v, want %v", got, tt.expected)
			}
		})
	}
}
