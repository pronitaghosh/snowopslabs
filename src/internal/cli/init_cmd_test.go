// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/capacity"
	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

func TestPreflightDocker(t *testing.T) {
	fake := func(info string, exit int) *toolchain.Fake {
		f := toolchain.NewFake()
		f.WhenArgsContain("context show", "colima\n", 0)
		f.WhenArgsContain("info", info, exit)
		return f
	}
	macOS := capacity.Platform{GOOS: "darwin"}
	tests := []struct {
		name       string
		fake       *toolchain.Fake
		wantErr    []string
		wantOutput string
	}{
		{name: "enough resources pass", fake: fake("2 4294967296", 0), wantOutput: "Docker has 2 CPU / 4 GB"},
		{
			name: "a running vm that is too small is refused with the resize",
			fake: fake("2 2054160384", 0),
			wantErr: []string{
				"too small", "2 CPU / 1.9 GB", "does not resize",
				"colima stop && colima start --cpu 2 --memory 4",
			},
		},
		{
			name:    "a stopped daemon is refused with the start command",
			fake:    fake("", 1),
			wantErr: []string{"not running", "colima start --cpu 2 --memory 4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := preflightDocker(t.Context(), &out, tt.fake, macOS)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(out.String(), tt.wantOutput) {
					t.Errorf("output = %q, want it to contain %q", out.String(), tt.wantOutput)
				}
				return
			}
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wantErr {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error should mention %q:\n%v", want, err)
				}
			}
		})
	}
}
