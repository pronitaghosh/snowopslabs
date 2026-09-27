// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"errors"
	"testing"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

func TestUsage(t *testing.T) {
	tests := []struct {
		name     string
		fake     func() *toolchain.Fake
		expected int
		wantErr  bool
	}{
		{
			name: "sums every lab container",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().
					WhenArgsContain("ps -q", "aaa\nbbb\nccc\n", 0).
					WhenArgsContain("stats", "973.1MiB / 3.814GiB\n1.2GiB / 3.814GiB\n512KiB / 3.814GiB\n", 0)
			},
			expected: 2203,
		},
		{
			name: "no containers uses nothing",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContain("ps -q", "", 0)
			},
			expected: 0,
		},
		{
			name: "docker failing is an error",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().WhenArgsContain("ps -q", "", 1)
			},
			wantErr: true,
		},
		{
			name: "unreadable stats are an error",
			fake: func() *toolchain.Fake {
				return toolchain.NewFake().
					WhenArgsContain("ps -q", "aaa\n", 0).
					WhenArgsContain("stats", "lots / 4GiB\n", 0)
			},
			wantErr: true,
		},
		{
			name: "docker not installed",
			fake: func() *toolchain.Fake {
				f := toolchain.NewFake()
				f.Available = map[string]string{}
				f.LookPathErr = errors.New("not found")
				return f
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Usage(t.Context(), tt.fake(), "k3d.cluster=snowops")
			if (err != nil) != tt.wantErr {
				t.Fatalf("Usage error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.expected {
				t.Errorf("Usage = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestParseDockerMemory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected float64
		wantErr  bool
	}{
		{name: "mebibytes", input: "973.5MiB", expected: 973.5},
		{name: "gibibytes", input: "1.5GiB", expected: 1536},
		{name: "kibibytes", input: "512KiB", expected: 0.5},
		{name: "zero bytes", input: "0B", expected: 0},
		{name: "no unit", input: "973", wantErr: true},
		{name: "not a number", input: "xMiB", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDockerMemory(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.expected {
				t.Errorf("parseDockerMemory(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
