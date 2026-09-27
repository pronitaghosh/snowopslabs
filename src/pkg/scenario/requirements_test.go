// SPDX-License-Identifier: Apache-2.0

package scenario

import "testing"

func TestRequirements_MemoryMiB(t *testing.T) {
	tests := []struct {
		name     string
		memory   string
		expected int
		wantErr  bool
	}{
		{name: "unset", memory: "", expected: 0},
		{name: "mebibytes", memory: "300Mi", expected: 300},
		{name: "fractional gibibytes", memory: "1.5Gi", expected: 1536},
		{name: "decimal megabytes round up", memory: "100M", expected: 96},
		{name: "decimal gigabytes", memory: "1G", expected: 954},
		{name: "kibibytes round up", memory: "1Ki", expected: 1},
		{name: "surrounding space", memory: " 256Mi ", expected: 256},
		{name: "no unit", memory: "512", wantErr: true},
		{name: "negative", memory: "-1Gi", wantErr: true},
		{name: "garbage", memory: "lots", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Requirements{Memory: tt.memory}.MemoryMiB()
			if (err != nil) != tt.wantErr {
				t.Fatalf("MemoryMiB() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.expected {
				t.Errorf("MemoryMiB() = %d, want %d", got, tt.expected)
			}
		})
	}
}

func TestRequirements_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     Requirements
		wantErr bool
	}{
		{name: "empty is valid", req: Requirements{}},
		{name: "full", req: Requirements{Memory: "1Gi", CPUs: 4, Agents: 2, Exclusive: true}},
		{name: "bad memory", req: Requirements{Memory: "a lot"}, wantErr: true},
		{name: "negative agents", req: Requirements{Agents: -1}, wantErr: true},
		{name: "negative cpus", req: Requirements{CPUs: -2}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate() = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
