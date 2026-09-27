// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

func TestPreflightDocker(t *testing.T) {
	fake := func(info string, exit int) *toolchain.Fake {
		f := toolchain.NewFake()
		f.WhenArgsContain("context show", "colima\n", 0)
		f.WhenArgsContain("info", info, exit)
		return f
	}
	tests := []struct {
		name    string
		profile string
		fake    *toolchain.Fake
		wantErr []string
	}{
		{name: "enough resources pass", profile: "k3d", fake: fake("2 4294967296", 0)},
		{
			name: "a running VM that is too small is refused with the resize", profile: "k3d",
			fake:    fake("2 2054160384", 0),
			wantErr: []string{"too small", "2 CPU / 1.9 GB", "does not resize", "colima stop && colima start --cpu 2 --memory 4"},
		},
		{
			name: "a stopped daemon is refused with the start command", profile: "kind",
			fake:    fake("", 1),
			wantErr: []string{"not running", "colima start --cpu 2 --memory 4"},
		},
		{name: "incluster has no Docker to check", profile: "incluster", fake: fake("", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := preflightDocker(context.Background(), tt.fake, tt.profile, "darwin", false)
			if len(tt.wantErr) == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
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
