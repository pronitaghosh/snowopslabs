// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

// Usage is the memory, in MiB, used now by the containers carrying label (for
// example "k3d.cluster=snowops"): the lab's live footprint.
func Usage(ctx context.Context, runner toolchain.Runner, label string) (int, error) {
	docker, err := runner.LookPath("docker")
	if err != nil {
		return 0, fmt.Errorf("finding docker: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	var ids bytes.Buffer
	if _, err := runner.Run(ctx, toolchain.Command{
		Path:   docker,
		Args:   []string{"ps", "-q", "--filter", "label=" + label},
		Stdout: &ids,
	}); err != nil {
		return 0, fmt.Errorf("listing the lab's containers: %w", err)
	}
	containers := strings.Fields(ids.String())
	if len(containers) == 0 {
		return 0, nil
	}

	var stats bytes.Buffer
	args := append([]string{"stats", "--no-stream", "--format", "{{.MemUsage}}"}, containers...)
	if _, err := runner.Run(ctx, toolchain.Command{Path: docker, Args: args, Stdout: &stats}); err != nil {
		return 0, fmt.Errorf("reading the lab's memory use: %w", err)
	}
	total := 0.0
	for line := range strings.Lines(stats.String()) {
		used, _, _ := strings.Cut(line, "/")
		mib, err := parseDockerMemory(strings.TrimSpace(used))
		if err != nil {
			return 0, err
		}
		total += mib
	}
	return int(math.Ceil(total)), nil
}

// parseDockerMemory reads a size as `docker stats` prints it ("973.1MiB",
// "1.2GiB", "512KiB", "0B") and returns MiB.
func parseDockerMemory(s string) (float64, error) {
	for _, u := range dockerMemoryUnits {
		num, ok := strings.CutSuffix(s, u.suffix)
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(num, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing memory %q from docker stats: %w", s, err)
		}
		return v * u.mib, nil
	}
	return 0, fmt.Errorf("unexpected memory %q from docker stats", s)
}

// dockerMemoryUnits maps `docker stats` suffixes to MiB. Longer suffixes come
// first so "MiB" is not read as "B".
var dockerMemoryUnits = []struct {
	suffix string
	mib    float64
}{
	{suffix: "GiB", mib: 1024},
	{suffix: "MiB", mib: 1},
	{suffix: "KiB", mib: 1.0 / 1024},
	{suffix: "GB", mib: 1e9 / (1 << 20)},
	{suffix: "MB", mib: 1e6 / (1 << 20)},
	{suffix: "kB", mib: 1e3 / (1 << 20)},
	{suffix: "B", mib: 1.0 / (1 << 20)},
}
