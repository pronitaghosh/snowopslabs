// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

// NodeLabel is the Docker label that selects a local cluster's node
// containers, and false for a runtime with none (incluster).
func NodeLabel(profile, cluster string) (string, bool) {
	switch profile {
	case "k3d":
		return "k3d.cluster=" + cluster, true
	case "kind":
		return "io.x-k8s.kind.cluster=" + cluster, true
	default:
		return "", false
	}
}

// Usage is the memory, in MiB, in use on the machine that runs the lab's
// containers: the Docker VM, or the host itself on native Linux. It is
// MemTotal minus MemAvailable from that machine's /proc/meminfo, read through
// one of the containers carrying label (e.g. "k3d.cluster=snowops"), so
// reclaimable page cache does not count and other workloads on the machine do.
// It returns 0 when the lab has no running container.
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

	var meminfo bytes.Buffer
	if _, err := runner.Run(ctx, toolchain.Command{
		Path:   docker,
		Args:   []string{"exec", containers[0], "cat", "/proc/meminfo"},
		Stdout: &meminfo,
	}); err != nil {
		return 0, fmt.Errorf("reading the Docker machine's memory: %w", err)
	}
	return usedMiB(meminfo.String())
}

// usedMiB reads MemTotal and MemAvailable (in kB) from /proc/meminfo and
// returns the difference in MiB.
func usedMiB(meminfo string) (int, error) {
	fields := map[string]int64{}
	sc := bufio.NewScanner(strings.NewReader(meminfo))
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		value := strings.Fields(rest)
		if len(value) == 0 {
			continue
		}
		kb, err := strconv.ParseInt(value[0], 10, 64)
		if err != nil {
			continue
		}
		fields[key] = kb
	}
	total, hasTotal := fields["MemTotal"]
	available, hasAvailable := fields["MemAvailable"]
	if !hasTotal || !hasAvailable {
		return 0, errors.New("/proc/meminfo has no MemTotal or MemAvailable")
	}
	return int((total - available) / 1024), nil
}
