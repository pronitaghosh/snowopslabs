// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

func TestMeets(t *testing.T) {
	tests := []struct {
		name string
		r    Resources
		n    Need
		want bool
	}{
		{"exactly the floor", Resources{2, 4 * gib}, Floor, true},
		{"8 GB colima VM reports 7.7 GiB", Resources{4, 8307675136}, Need{4, 8}, true},
		{"default 2 GB colima VM", Resources{2, 2054160384}, Floor, false},
		{"too few CPUs", Resources{1, 16 * gib}, Floor, false},
		{"just under the tolerance", Resources{2, 3972844748}, Floor, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.Meets(tt.n); got != tt.want {
				t.Errorf("Meets = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProbe(t *testing.T) {
	tests := []struct {
		name    string
		fake    func() *toolchain.Fake
		want    Resources
		wantErr error
	}{
		{
			name: "reads CPU and memory",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContain("info", "4 8307675136\n", 0)
			},
			want: Resources{4, 8307675136},
		},
		{
			name: "docker not installed",
			fake: func() *toolchain.Fake {
				f := toolchain.NewFake()
				f.Available = map[string]string{"kubectl": "/usr/bin/kubectl"}
				f.LookPathErr = errors.New("not found")
				return f
			},
			wantErr: ErrDockerMissing,
		},
		{
			name: "daemon down",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContain("info", "", 1)
			},
			wantErr: ErrDaemonDown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Probe(context.Background(), tt.fake())
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	t.Run("garbled output is an error, not a zero", func(t *testing.T) {
		f := toolchain.NewFake().WhenArgsContain("info", "lots of text\n", 0)
		if _, err := Probe(context.Background(), f); err == nil ||
			errors.Is(err, ErrDaemonDown) {
			t.Fatalf("want a parse error, got %v", err)
		}
	})
}

func TestDetectHost(t *testing.T) {
	tests := []struct {
		name string
		goos string
		wsl  bool
		fake func() *toolchain.Fake
		want Engine
	}{
		{"wsl wins", "linux", true, toolchain.NewFake, EngineWSL},
		{"native linux", "linux", false, toolchain.NewFake, EngineNative},
		{"colima context", "darwin", false, func() *toolchain.Fake {
			return toolchain.NewFake().WhenArgsContain("context show", "colima\n", 0)
		}, EngineColima},
		{"named colima profile", "darwin", false, func() *toolchain.Fake {
			return toolchain.NewFake().WhenArgsContain("context show", "colima-work\n", 0)
		}, EngineColima},
		{"docker desktop context", "darwin", false, func() *toolchain.Fake {
			return toolchain.NewFake().WhenArgsContain("context show", "desktop-linux\n", 0)
		}, EngineDockerDesktop},
		{"colima installed but never started", "darwin", false, func() *toolchain.Fake {
			f := toolchain.NewFake().WhenArgsContain("context show", "default\n", 0)
			f.Available = map[string]string{"docker": "/opt/homebrew/bin/docker", "colima": "/opt/homebrew/bin/colima"}
			f.LookPathErr = errors.New("not found")
			return f
		}, EngineColima},
		{"default context, no colima", "darwin", false, func() *toolchain.Fake {
			f := toolchain.NewFake().WhenArgsContain("context show", "default\n", 0)
			f.Available = map[string]string{"docker": "/usr/local/bin/docker"}
			f.LookPathErr = errors.New("not found")
			return f
		}, EngineDockerDesktop},
		{"only colima installed", "darwin", false, func() *toolchain.Fake {
			f := toolchain.NewFake()
			f.Available = map[string]string{"colima": "/opt/homebrew/bin/colima"}
			f.LookPathErr = errors.New("not found")
			return f
		}, EngineColima},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectHost(context.Background(), tt.fake(), tt.goos, tt.wsl).Engine; got != tt.want {
				t.Errorf("engine = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHints(t *testing.T) {
	n := Need{CPUs: 4, MemGiB: 6}
	tests := []struct {
		engine     Engine
		wantResize string
		wantStart  string
	}{
		{EngineColima, "colima stop && colima start --cpu 4 --memory 6", "colima start --cpu 4 --memory 6"},
		{EngineDockerDesktop, "Settings → Resources → CPUs 4, Memory 6 GB", "open -a Docker"},
		{EngineWSL, "processors=4 and memory=6GB", "sudo service docker start"},
		{EngineNative, "needs 4 CPUs and 6 GB free", "sudo systemctl start docker"},
	}
	for _, tt := range tests {
		h := Host{Engine: tt.engine}
		if got := h.ResizeHint(n); !strings.Contains(got, tt.wantResize) {
			t.Errorf("engine %v resize = %q, want it to contain %q", tt.engine, got, tt.wantResize)
		}
		if got := h.StartHint(n); !strings.Contains(got, tt.wantStart) {
			t.Errorf("engine %v start = %q, want it to contain %q", tt.engine, got, tt.wantStart)
		}
	}
}

func TestShortfallFormatting(t *testing.T) {
	got := Shortfall(Resources{2, 2054160384}, Floor)
	want := "Docker has 2 CPU / 1.9 GB; this needs at least 2 CPU / 4 GB."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if Shortfall(Resources{4, 16 * gib}, Floor) != "" {
		t.Error("a machine that meets the need has no shortfall")
	}
}

func TestParseGiB(t *testing.T) {
	tests := []struct {
		in   string
		want float64
	}{{"4", 4}, {"6GB", 6}, {"8GiB", 8}, {" 5g ", 5}, {"4.5", 4.5}}
	for _, tt := range tests {
		in, want := tt.in, tt.want
		got, err := ParseGiB(in)
		if err != nil || got != want {
			t.Errorf("ParseGiB(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "lots", "0", "-2GB"} {
		if _, err := ParseGiB(bad); err == nil {
			t.Errorf("ParseGiB(%q) should fail", bad)
		}
	}
}
