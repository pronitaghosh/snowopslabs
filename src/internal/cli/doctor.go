// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sagar2395/snowopslabs/internal/capacity"
	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

// doctorCmd checks the user's environment before a cluster build. Each
// failure names the tool, what breaks without it, and how to fix it on this
// platform.
func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check that your environment can run SnowOps Labs",
		Long: `Verifies every external tool SnowOps Labs depends on: that it is
installed, that it is new enough, and that the cluster is reachable.

Each problem is reported with the reason it matters and how to fix it. Exits
non-zero if anything required is missing or out of date, so it is safe to use
as a gate in a script.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDoctor(cmd.Context(), cmd.OutOrStdout(), toolchain.NewExec())
		},
	}
}

// runDoctor runs the checks and prints the results to out. It takes a runner
// so tests can use a fake.
func runDoctor(ctx context.Context, out io.Writer, runner toolchain.Runner) error {
	if ctx == nil {
		ctx = context.Background()
	}

	results, err := toolchain.NewPreflight(runner).Check(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "SnowOps Labs environment check")
	fmt.Fprintln(out)

	w := tabwriter.NewWriter(out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOOL\tSTATUS\tVERSION\tREQUIRED")
	for _, r := range results {
		version := r.Version
		if version == "" {
			version = "-"
		}
		required := r.Required
		if required == "" {
			required = "any"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Binary, statusLabel(r), version, required)
	}
	_ = w.Flush()

	var problems, notes []string
	for _, r := range results {
		if r.Detail == "" {
			continue
		}
		if r.OK() {
			if r.Status != toolchain.CheckOK {
				notes = append(notes, r.Detail)
			}
			continue
		}
		problems = append(problems, r.Detail)
	}

	dockerLine, dockerProblem, dockerNote := dockerCapacity(ctx, runner, runtime.GOOS, isWSL())
	if dockerLine != "" {
		fmt.Fprintf(out, "\nDocker:   %s\n", dockerLine)
	}
	if dockerProblem != "" {
		problems = append(problems, dockerProblem)
	}
	if dockerNote != "" {
		notes = append(notes, dockerNote)
	}
	if !hostsBlockPresent() {
		notes = append(notes, "Ingress hostnames (e.g. http://grafana.k3d.local) won't resolve until you\n"+
			"    run 'labctl hosts add' (one-time, needs sudo). Not needed for the UI at :3939.")
	}

	if len(notes) > 0 {
		fmt.Fprintln(out, "\nNotes:")
		for _, n := range notes {
			fmt.Fprintf(out, "  - %s\n", n)
		}
	}
	if isWSL() {
		fmt.Fprintln(out, "\nWSL notes:")
		cwd, _ := os.Getwd()
		for _, n := range wslDoctorNotes(cwd) {
			fmt.Fprintf(out, "  - %s\n", n)
		}
	}

	if len(problems) > 0 {
		fmt.Fprintln(out, "\nProblems to fix:")
		for _, p := range problems {
			fmt.Fprintf(out, "  ✗ %s\n", p)
		}
		fmt.Fprintln(out)
		return fmt.Errorf("%d problem(s) to fix before SnowOps Labs can run", len(problems))
	}

	if len(notes) > 0 {
		fmt.Fprintln(out, "\n✓ Ready to run SnowOps Labs (see the notes above).")
	} else {
		fmt.Fprintln(out, "\n✓ Ready to run SnowOps Labs.")
	}
	return nil
}

func statusLabel(r toolchain.CheckResult) string {
	switch r.Status {
	case toolchain.CheckOK:
		return "ok"
	case toolchain.CheckMissing:
		if r.Optional {
			return "missing (optional)"
		}
		return "MISSING"
	case toolchain.CheckOutdated:
		if r.Optional {
			return "outdated (optional)"
		}
		return "OUTDATED"
	default:
		return "unknown"
	}
}

// dockerCapacity checks the Docker engine against the lab's minimum. It
// returns a summary line for the report, plus at most one problem (blocks the
// lab) or note (worth knowing). A missing docker is left to the tool checks.
func dockerCapacity(ctx context.Context, runner toolchain.Runner, goos string, wsl bool) (line, problem, note string) {
	host := capacity.DetectHost(ctx, runner, goos, wsl)
	res, err := capacity.Probe(ctx, runner)
	switch {
	case errors.Is(err, capacity.ErrDockerMissing):
		return "", "", ""
	case errors.Is(err, capacity.ErrNoPermission), errors.Is(err, capacity.ErrWSLIntegration):
		return "not usable", fmt.Sprintf("%v. Fix:\n    %s", err, host.AccessHint(err)), ""
	case errors.Is(err, capacity.ErrDaemonDown):
		if host.Engine == capacity.EngineColima {
			return "not running", "", fmt.Sprintf(
				"colima is not running. 'labctl init' starts it for you at %s, or run:\n    %s",
				capacity.Floor, host.StartHint(capacity.Floor))
		}
		return "not running", fmt.Sprintf(
			"The Docker daemon is not running. Start it:\n    %s", host.StartHint(capacity.Floor)), ""
	case err != nil:
		return "unknown", "", fmt.Sprintf("Could not read Docker's CPU and memory: %v", err)
	}
	if short := capacity.Shortfall(res, capacity.Floor); short != "" {
		return res.String(), fmt.Sprintf(
			"%s\n    Below this the API server runs out of memory and every command fails with\n"+
				"    \"TLS handshake timeout\". Fix:\n    %s", short, host.ResizeHint(capacity.Floor)), ""
	}
	return fmt.Sprintf("%s — meets the minimum (%s). Heavier scenarios say what they need.",
		res, capacity.Floor), "", ""
}

func init() {
	rootCmd.AddCommand(doctorCmd())
}
