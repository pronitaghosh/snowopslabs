// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sagar2395/snowopslabs/internal/capacity"
	"github.com/sagar2395/snowopslabs/internal/config"
	"github.com/sagar2395/snowopslabs/internal/k8s"
	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the lab (setup tools + create cluster + install platform)",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("=== Setting up tools ===")
		if err := scriptExec.RunScript("bootstrap/setup-tools.sh", cfg.Profile); err != nil {
			return fmt.Errorf("setup-tools failed: %w", err)
		}

		if err := preflightDocker(cmdContext(cmd), toolchain.NewExec(), cfg.Profile, runtime.GOOS, isWSL()); err != nil {
			return err
		}

		fmt.Println("\n=== Creating runtime ===")
		if err := scriptExec.RunScript(
			fmt.Sprintf("runtimes/%s/up.sh", cfg.Profile),
			cfg.ClusterName,
		); err != nil {
			return fmt.Errorf("creating the cluster failed: %w", err)
		}

		fmt.Println("\n=== Installing platform ===")
		if err := platformUpRun(cmd, args); err != nil {
			return err
		}
		if err := checkClusterHealthy(cmdContext(cmd)); err != nil {
			return err
		}
		printPostInitHints()
		return nil
	},
}

// preflightDocker refuses to build the lab on a Docker engine that cannot hold
// it. setup-tools has already started a stopped colima at LAB_CPUS/LAB_MEMORY,
// so a failure here is a daemon that will not start or one the user runs at a
// smaller size — which labctl does not resize, since restarting it would stop
// the user's other containers.
func preflightDocker(ctx context.Context, runner toolchain.Runner, profile, goos string, wsl bool) error {
	if profile == "incluster" {
		return nil
	}
	fmt.Println("\n=== Checking Docker resources ===")
	host := capacity.DetectHost(ctx, runner, goos, wsl)
	res, err := capacity.Probe(ctx, runner)
	switch {
	case errors.Is(err, capacity.ErrDockerMissing), errors.Is(err, capacity.ErrDaemonDown):
		return fmt.Errorf("%w.\nStart it, then re-run 'labctl init':\n  %s", err, host.StartHint(capacity.Floor))
	case err != nil:
		return fmt.Errorf("reading Docker's CPU and memory: %w", err)
	}
	if short := capacity.Shortfall(res, capacity.Floor); short != "" {
		return fmt.Errorf("docker is too small for the lab. %s\n"+
			"labctl does not resize a running Docker engine, because that would stop your other containers.\n"+
			"Resize it, then re-run 'labctl init':\n  %s", short, host.ResizeHint(capacity.Floor))
	}
	fmt.Printf("Docker has %s (minimum %s).\n", res, capacity.Floor)
	return nil
}

// checkClusterHealthy is init's last step: it only reports "Lab is up" once
// the API answers and every node is Ready.
func checkClusterHealthy(ctx context.Context) error {
	fmt.Println("\n=== Checking the lab ===")
	if err := k8s.Reachable(ctx); err != nil {
		return fmt.Errorf("the cluster is not answering (%v).\n"+
			"This is almost always Docker running out of memory. Check with 'labctl doctor', then re-run 'labctl init'", err)
	}
	wctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if _, err := k8s.RunKubectl(wctx, "wait", "--for=condition=Ready", "node", "--all", "--timeout=150s"); err != nil {
		return fmt.Errorf("not every node became Ready: %w\nSee which with 'kubectl get nodes', then re-run 'labctl init'", err)
	}
	fmt.Println("API server ready; all nodes Ready.")
	return nil
}

// cmdContext is the command's context, or Background when it has none (as
// when a test calls RunE directly).
func cmdContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func printPostInitHints() {
	suffix := cfg.DomainSuffix
	if suffix == "" {
		suffix = "k3d.local"
	}
	fmt.Println("\n=== Lab is up ===")
	if !hostsBlockPresent() {
		fmt.Printf("\nTo open ingress URLs like http://grafana.%s in your browser, add the\n", suffix)
		fmt.Println("hostnames to /etc/hosts (one-time, needs sudo):")
		fmt.Println("  labctl hosts add")
		fmt.Println("(The labctl UI itself needs no hosts entry: http://localhost:3939)")
	}
	fmt.Println("\nNext: deploy an app, then run a scenario:")
	fmt.Println("  labctl app build go-api && labctl app deploy go-api")
	fmt.Println("  labctl scenario up observability-sre   (add --deploy-prereqs to auto-deploy its apps)")
}

var teardownCmd = &cobra.Command{
	Use:   "teardown",
	Short: "Tear down the lab (deactivate scenarios/incidents + destroy apps + platform + cluster)",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Deactivate scenarios and incidents before removing the platform and
		// apps they depend on.
		fmt.Println("=== Deactivating scenarios and incidents ===")
		if cleared := scenes.DeactivateAll(); len(cleared) > 0 {
			fmt.Printf("Deactivated scenarios: %s\n", strings.Join(cleared, ", "))
		}
		if incEng != nil {
			if fault := incEng.ClearActiveState(); fault != "" {
				fmt.Printf("Deactivated incident: %s\n", fault)
			}
		}

		fmt.Println("\n=== Destroying all apps ===")
		apps, _ := config.ListApps(cfg.ProjectRoot)
		for _, app := range apps {
			fmt.Printf("Destroying %s...\n", app)
			_ = scriptExec.RunScript("src/engine/deploy.sh", "destroy", app)
		}

		fmt.Println("\n=== Removing platform ===")
		_ = platformDownRun(cmd, args)

		fmt.Println("\n=== Destroying runtime ===")
		return scriptExec.RunScript(fmt.Sprintf("runtimes/%s/down.sh", cfg.Profile))
	},
}

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Reset the lab (teardown + init)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := teardownCmd.RunE(cmd, args); err != nil {
			fmt.Printf("Warning during teardown: %v\n", err)
		}
		return initCmd.RunE(cmd, args)
	},
}

var setupToolsCmd = &cobra.Command{
	Use:   "setup-tools",
	Short: "Install the required CLI tools (kubectl, helm, k3d/kind, container runtime)",
	Long: `Install the tools SnowOps Labs needs, version-pinned from versions.env, for the
active PROFILE. This is the same step 'labctl init' runs first; use it on its own
to prepare a machine without creating a cluster. Equivalent to 'make setup-tools'.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return scriptExec.RunScript("bootstrap/setup-tools.sh", cfg.Profile)
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(teardownCmd)
	rootCmd.AddCommand(resetCmd)
	rootCmd.AddCommand(setupToolsCmd)
}
