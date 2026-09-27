// SPDX-License-Identifier: Apache-2.0

// Package capacity reads how much CPU and memory the Docker engine has and
// compares it with what the lab needs. doctor, init's preflight gate and the
// activation gates all go through it, so the numbers and the fix they print
// never disagree.
package capacity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

// Floor is the smallest Docker engine the lab supports: the cluster plus the
// baseline platform (ingress, Prometheus, Grafana). Below it the API server
// runs out of memory and every call fails with "TLS handshake timeout".
var Floor = Need{CPUs: 2, MemGiB: 4}

// memTolerance lets a VM sized at N GiB meet an N GiB requirement: the guest
// kernel reserves part of it, so `colima start --memory 8` reports ~7.7 GiB.
const memTolerance = 0.95

const gib = 1 << 30

var (
	// ErrDockerMissing means the docker CLI is not on PATH.
	ErrDockerMissing = errors.New("docker is not installed")
	// ErrDaemonDown means the CLI is present but the engine does not answer.
	ErrDaemonDown = errors.New("the Docker daemon is not running")
	// ErrNoPermission means the daemon answered but refused this user: they
	// are not in the docker group yet, or their session predates joining it.
	ErrNoPermission = errors.New("the Docker daemon refused this user (permission denied on its socket)")
	// ErrWSLIntegration means Docker Desktop's WSL shim is present but
	// integration is off for this distro.
	ErrWSLIntegration = errors.New("this WSL distro has Docker Desktop integration turned off")
)

// Need is a CPU and memory requirement for the Docker engine.
type Need struct {
	CPUs   int
	MemGiB float64
}

func (n Need) String() string {
	return fmt.Sprintf("%d CPU / %s GB", n.CPUs, formatGiB(n.MemGiB))
}

// Resources is what the Docker engine reports it has.
type Resources struct {
	CPUs     int
	MemBytes int64
}

// MemGiB is the engine's memory in GiB.
func (r Resources) MemGiB() float64 { return float64(r.MemBytes) / gib }

func (r Resources) String() string {
	return fmt.Sprintf("%d CPU / %s GB", r.CPUs, formatGiB(r.MemGiB()))
}

// Meets reports whether the engine satisfies n.
func (r Resources) Meets(n Need) bool {
	return r.CPUs >= n.CPUs && r.MemGiB() >= n.MemGiB*memTolerance
}

// Probe asks the Docker engine for its CPU count and memory. It returns
// ErrDockerMissing or ErrDaemonDown when the engine cannot be asked.
func Probe(ctx context.Context, runner toolchain.Runner) (Resources, error) {
	path, err := runner.LookPath("docker")
	if err != nil {
		return Resources{}, ErrDockerMissing
	}
	// A hung daemon must not hang the caller.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var buf, errBuf bytes.Buffer
	if _, err := runner.Run(ctx, toolchain.Command{
		Path:   path,
		Args:   []string{"info", "--format", "{{.NCPU}} {{.MemTotal}}"},
		Stdout: &buf,
		Stderr: &errBuf,
	}); err != nil {
		msg := errBuf.String() + buf.String()
		switch {
		case strings.Contains(msg, "could not be found in this WSL 2 distro"):
			return Resources{}, ErrWSLIntegration
		case strings.Contains(msg, "permission denied"):
			return Resources{}, ErrNoPermission
		}
		return Resources{}, ErrDaemonDown
	}
	fields := strings.Fields(buf.String())
	if len(fields) != 2 {
		return Resources{}, fmt.Errorf("unexpected `docker info` output %q", buf.String())
	}
	ncpu, err := strconv.Atoi(fields[0])
	if err != nil || ncpu <= 0 {
		return Resources{}, fmt.Errorf("unexpected CPU count %q from `docker info`", fields[0])
	}
	mem, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || mem <= 0 {
		return Resources{}, fmt.Errorf("unexpected memory %q from `docker info`", fields[1])
	}
	return Resources{CPUs: ncpu, MemBytes: mem}, nil
}

// Engine is where Docker runs, which decides how it is started and resized.
type Engine int

const (
	// EngineNative is Docker Engine running directly on a Linux host.
	EngineNative Engine = iota
	// EngineColima is the colima VM on macOS.
	EngineColima
	// EngineDockerDesktop is Docker Desktop on macOS.
	EngineDockerDesktop
	// EngineWSL is any Docker inside WSL2; its memory comes from .wslconfig.
	EngineWSL
)

// Host describes the machine for the purpose of printing a fix.
type Host struct {
	Engine Engine
}

// DetectHost works out which engine the user has. goos and wsl come from the
// caller so tests need not run on each platform.
func DetectHost(ctx context.Context, runner toolchain.Runner, goos string, wsl bool) Host {
	switch {
	case wsl:
		return Host{Engine: EngineWSL}
	case goos != "darwin":
		return Host{Engine: EngineNative}
	}
	// colima registers a "colima" docker context, which stays selected while
	// the VM is stopped, so this also recognises a stopped colima.
	if path, err := runner.LookPath("docker"); err == nil {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var buf bytes.Buffer
		if _, err := runner.Run(ctx, toolchain.Command{
			Path: path, Args: []string{"context", "show"}, Stdout: &buf,
		}); err == nil {
			switch name := strings.TrimSpace(buf.String()); {
			case strings.HasPrefix(name, "colima"):
				return Host{Engine: EngineColima}
			case strings.HasPrefix(name, "desktop"):
				return Host{Engine: EngineDockerDesktop}
			}
		}
	}
	// "default" context: a colima that has never been started has not yet
	// registered its context, so an installed colima is the better guess.
	if _, err := runner.LookPath("colima"); err == nil {
		return Host{Engine: EngineColima}
	}
	return Host{Engine: EngineDockerDesktop}
}

// StartHint is the command that starts a stopped engine at size n.
func (h Host) StartHint(n Need) string {
	switch h.Engine {
	case EngineColima:
		return fmt.Sprintf("colima start --cpu %d --memory %s", n.CPUs, formatGiB(n.MemGiB))
	case EngineDockerDesktop:
		return "open -a Docker   (then wait for Docker Desktop to report it is running)"
	case EngineWSL:
		return "sudo service docker start   (or, with Docker Desktop, start it on Windows and enable WSL Integration for this distro)"
	default:
		return "sudo systemctl start docker"
	}
}

// AccessHint is the fix for ErrNoPermission and ErrWSLIntegration.
func (h Host) AccessHint(err error) string {
	if errors.Is(err, ErrWSLIntegration) {
		return "Docker Desktop → Settings → Resources → WSL Integration → enable this distro,\n" +
			"Apply & Restart, then reopen this terminal"
	}
	relogin := "log out and back in (or run `newgrp docker`)"
	if h.Engine == EngineWSL {
		relogin = "run `wsl --shutdown` in PowerShell and reopen this terminal"
	}
	return "sudo usermod -aG docker \"$USER\", then " + relogin
}

// ResizeHint explains how to give the engine at least n.
func (h Host) ResizeHint(n Need) string {
	mem := formatGiB(n.MemGiB)
	switch h.Engine {
	case EngineColima:
		return fmt.Sprintf("colima stop && colima start --cpu %d --memory %s", n.CPUs, mem)
	case EngineDockerDesktop:
		return fmt.Sprintf("Docker Desktop → Settings → Resources → CPUs %d, Memory %s GB → Apply & Restart", n.CPUs, mem)
	case EngineWSL:
		return fmt.Sprintf("in %%UserProfile%%\\.wslconfig on Windows set [wsl2] processors=%d and memory=%sGB,\n"+
			"then run `wsl --shutdown` in PowerShell and reopen this terminal", n.CPUs, mem)
	default:
		return fmt.Sprintf("Docker uses this machine's own CPU and memory: it needs %d CPUs and %s GB free —\n"+
			"close other memory-hungry programs, or use a bigger machine", n.CPUs, mem)
	}
}

// Shortfall is the one-line statement of what is missing, or "" when r meets n.
func Shortfall(r Resources, n Need) string {
	if r.Meets(n) {
		return ""
	}
	return fmt.Sprintf("Docker has %s; this needs at least %s.", r, n)
}

// ParseGiB reads a memory size such as "4", "4G", "4GB" or "4GiB".
func ParseGiB(s string) (float64, error) {
	t := strings.TrimSpace(strings.ToUpper(s))
	for _, suffix := range []string{"GIB", "GB", "G"} {
		t = strings.TrimSuffix(t, suffix)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("memory %q: want a size in GB such as 4 or 6GB", s)
	}
	return v, nil
}

// formatGiB prints "4" for whole numbers and "7.7" otherwise.
func formatGiB(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}
