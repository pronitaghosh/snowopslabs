// SPDX-License-Identifier: Apache-2.0

package scenario

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Requirements is what activating a scenario or fault needs from the lab
// beyond what it already declares elsewhere. Helm releases, prerequisite
// platform components and apps have their own measured footprints
// (config/footprints.yaml), so shared ones are counted once however many
// scenarios use them; Memory covers only the workloads the item itself runs.
type Requirements struct {
	// Memory is what the item's own workloads use, as a Kubernetes quantity
	// such as "300Mi" or "1Gi".
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"`
	// CPUs is the fewest Docker CPUs it runs well on. Fewer only warns: CPU
	// shortage makes the lab slow, where memory shortage breaks it.
	CPUs int `yaml:"cpus,omitempty" json:"cpus,omitempty"`
	// Agents is how many agent nodes it needs, such as 2 for a drill that
	// drains a node while the app keeps serving. k3d adds them live.
	Agents int `yaml:"agents,omitempty" json:"agents,omitempty"`
	// Exclusive means nothing else may be active alongside it, as for a drill
	// that replaces the cluster's nodes.
	Exclusive bool `yaml:"exclusive,omitempty" json:"exclusive,omitempty"`
}

// MemoryMiB is Memory in MiB, or 0 when unset.
func (r Requirements) MemoryMiB() (int, error) {
	if strings.TrimSpace(r.Memory) == "" {
		return 0, nil
	}
	return ParseMemoryMiB(r.Memory)
}

// Validate reports a malformed requirement.
func (r Requirements) Validate() error {
	if _, err := r.MemoryMiB(); err != nil {
		return err
	}
	if r.CPUs < 0 || r.Agents < 0 {
		return errors.New("requirements: cpus and agents cannot be negative")
	}
	return nil
}

// ParseMemoryMiB reads a Kubernetes memory quantity ("512Mi", "1.5Gi",
// "300M", "1G") and returns it in MiB, rounded up.
func ParseMemoryMiB(q string) (int, error) {
	s := strings.TrimSpace(q)
	for _, u := range memoryUnits {
		num, ok := strings.CutSuffix(s, u.suffix)
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, fmt.Errorf("requirements: memory %q is not a quantity such as 300Mi or 1Gi: %w", q, err)
		}
		if v < 0 {
			return 0, fmt.Errorf("requirements: memory %q cannot be negative", q)
		}
		return int(math.Ceil(v * u.mib)), nil
	}
	return 0, fmt.Errorf("requirements: memory %q is not a quantity such as 300Mi or 1Gi", q)
}

// memoryUnits maps each quantity suffix to its size in MiB. Two-letter binary
// suffixes come first so "Mi" is not read as "M".
var memoryUnits = []struct {
	suffix string
	mib    float64
}{
	{suffix: "Gi", mib: 1024},
	{suffix: "Mi", mib: 1},
	{suffix: "Ki", mib: 1.0 / 1024},
	{suffix: "G", mib: 1e9 / (1 << 20)},
	{suffix: "M", mib: 1e6 / (1 << 20)},
	{suffix: "K", mib: 1e3 / (1 << 20)},
}
