// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/sagar2395/snowopslabs/internal/capacity"
	"github.com/sagar2395/snowopslabs/internal/config"
	"github.com/sagar2395/snowopslabs/internal/k8s"
	"github.com/sagar2395/snowopslabs/internal/service/admission"
	"github.com/sagar2395/snowopslabs/internal/toolchain"
)

// newAdmissionGate builds the capacity gate for the configured lab, or nil for
// a runtime whose memory is not the Docker engine's (incluster).
func newAdmissionGate() *admission.Gate {
	label, ok := nodeContainerLabel(cfg.Profile, cfg.ClusterName)
	if !ok {
		return nil
	}
	footprints, err := capacity.LoadFootprints(filepath.Join(cfg.ProjectRoot, "config", "footprints.yaml"))
	if err != nil {
		slog.Warn("capacity footprints unreadable; judging by live usage only", "err", err)
	}
	return &admission.Gate{
		Footprints:   footprints,
		Runner:       toolchain.NewExec(),
		Platform:     capacity.Platform{GOOS: runtime.GOOS, WSL: isWSL()},
		UsageLabel:   label,
		CanAddAgents: cfg.Profile == "k3d",
		Active:       scenes.ActiveDemands,
		Running:      itemRunning,
		Agents:       k8s.AgentCount,
		AddAgents: func(_ context.Context, total int) error {
			return scriptExec.RunScript(filepath.Join("runtimes", cfg.Profile, "add-agents.sh"), strconv.Itoa(total))
		},
	}
}

// nodeContainerLabel is the Docker label that selects a local cluster's node
// containers, and false for a runtime with none.
func nodeContainerLabel(profile, cluster string) (string, bool) {
	switch profile {
	case "k3d":
		return "k3d.cluster=" + cluster, true
	case "kind":
		return "io.x-k8s.kind.cluster=" + cluster, true
	default:
		return "", false
	}
}

// itemRunning reports whether a release, platform component or app already
// runs in the cluster, so the capacity check does not charge for it twice.
func itemRunning(ctx context.Context, item capacity.Item) bool {
	switch item.Kind {
	case capacity.ItemRelease:
		ns, name, ok := strings.Cut(item.Name, "/")
		return ok && k8s.HelmReleaseDeployed(ctx, ns, name)
	case capacity.ItemPlatform:
		category, provider, err := resolveTarget(item.Name)
		if err != nil {
			return false
		}
		p, err := reg.GetProvider(category, provider)
		if err != nil {
			return false
		}
		if p.SharesNamespace() {
			return k8s.HelmReleaseExists(ctx, p.Namespace(), p.Name)
		}
		return k8s.NamespaceExists(ctx, p.Namespace())
	case capacity.ItemApp:
		ns := item.Name
		if appCfg, err := config.LoadAppConfig(cfg.ProjectRoot, item.Name); err == nil && appCfg.Namespace != "" {
			ns = appCfg.Namespace
		}
		status, err := k8s.GetAppStatus(ctx, item.Name, ns)
		return err == nil && status != nil && status.Deployed
	default:
		return false
	}
}

// attachAdmissionGate attaches the capacity gate to both engines, so every activation
// path (CLI, web UI, challenges) is checked the same way.
func attachAdmissionGate() {
	gate := newAdmissionGate()
	if gate == nil {
		return
	}
	scenes.Admit = gate.Admit
	incEng.Admit = gate.Admit
}
