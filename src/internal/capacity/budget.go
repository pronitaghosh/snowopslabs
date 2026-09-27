// SPDX-License-Identifier: Apache-2.0

package capacity

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// usableShare is the part of the Docker engine's memory the lab may plan to
// use. The rest absorbs start-up spikes, page cache and the engine itself.
const usableShare = 0.85

// Footprints are the measured memory costs, in MiB, of what the lab can run.
// They live in config/footprints.yaml.
type Footprints struct {
	// Baseline is the cluster with one agent plus the platform init installs.
	Baseline int `yaml:"baseline"`
	// Agent is each agent node beyond the first.
	Agent int `yaml:"agent"`
	// Releases are Helm releases scenarios install, by release name.
	Releases map[string]int `yaml:"releases"`
	// Platform are platform components, by "category/provider".
	Platform map[string]int `yaml:"platform"`
	// Apps are deployable apps, by name.
	Apps map[string]int `yaml:"apps"`
	// Unlisted is the cost assumed for anything not listed above.
	Unlisted int `yaml:"unlisted"`
}

// LoadFootprints reads footprints from path. A missing file is not an error:
// the zero Footprints costs every item at 0 and only live usage counts.
func LoadFootprints(file string) (Footprints, error) {
	data, err := os.ReadFile(file) //nolint:gosec // file is the project's own config/footprints.yaml
	if errors.Is(err, fs.ErrNotExist) {
		return Footprints{}, nil
	}
	if err != nil {
		return Footprints{}, fmt.Errorf("reading footprints: %w", err)
	}
	var f Footprints
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Footprints{}, fmt.Errorf("parsing %s: %w", file, err)
	}
	return f, nil
}

// cost is the footprint of item, or Unlisted when it has none.
func (f Footprints) cost(item Item) int {
	var table map[string]int
	key := item.Name
	switch item.Kind {
	case ItemRelease:
		table, key = f.Releases, path.Base(item.Name)
	case ItemPlatform:
		table = f.Platform
	case ItemApp:
		table = f.Apps
	default:
		return 0
	}
	if mib, ok := table[key]; ok {
		return mib
	}
	return f.Unlisted
}

// ItemKind says what an Item is, which decides where its footprint is found.
type ItemKind int

const (
	// ItemUnknown is the zero value and costs nothing.
	ItemUnknown ItemKind = iota
	// ItemRelease is a Helm release, named "namespace/release".
	ItemRelease
	// ItemPlatform is a platform component, named "category/provider".
	ItemPlatform
	// ItemApp is a deployable app, named after its apps/ directory.
	ItemApp
)

// Item is one shared thing an activation runs. Two activations that use the
// same Item share it, so it is counted once.
type Item struct {
	Kind ItemKind
	Name string
}

// Demand is what one scenario or fault adds to the lab.
type Demand struct {
	// Name identifies the scenario or fault in messages.
	Name string
	// Items are the shared releases, platform components and apps it uses.
	Items []Item
	// OwnMiB is memory for workloads that belong to it alone.
	OwnMiB int
	// CPUs is the fewest Docker CPUs it runs well on.
	CPUs int
	// Agents is how many agent nodes it needs.
	Agents int
	// Exclusive means nothing else may be active alongside it.
	Exclusive bool
	// App is the workload it is bound to.
	App string
}

// Lab is the state an activation is judged against.
type Lab struct {
	// Engine is what the Docker engine has.
	Engine Resources
	// Host decides how a resize is worded.
	Host Host
	// UsedMiB is what the lab's node containers use now; 0 means unknown.
	UsedMiB int
	// Running holds the items already running, which cost nothing more.
	Running map[Item]bool
	// Agents is the cluster's current agent count.
	Agents int
	// CanAddAgents reports whether the runtime can add agent nodes live.
	CanAddAgents bool
}

// Verdict is the outcome of Evaluate.
type Verdict struct {
	// Problems block the activation; each says how to resolve it.
	Problems []string
	// Warnings are worth knowing but do not block.
	Warnings []string
	// NeedMiB is the memory the lab would use with the activation.
	NeedMiB int
	// LimitMiB is the memory it may plan to use.
	LimitMiB int
	// AddAgents is how many agent nodes to add before activating.
	AddAgents int
}

// Allowed reports whether the activation may go ahead.
func (v Verdict) Allowed() bool { return len(v.Problems) == 0 }

// Evaluate decides whether next can be activated alongside active. Memory is
// judged twice and the larger figure wins: once from footprints alone (so a
// scenario that is still starting is counted in full), and once from live
// usage plus what next would start (so anything else running is counted).
func Evaluate(f Footprints, lab Lab, active []Demand, next Demand) Verdict {
	v := Verdict{Problems: []string{}, Warnings: []string{}}
	v.checkExclusive(active, next)

	agents := max(lab.Agents, next.Agents)
	if next.Agents > lab.Agents {
		if lab.CanAddAgents {
			v.AddAgents = next.Agents - lab.Agents
		} else {
			v.Problems = append(v.Problems, fmt.Sprintf(
				"%s needs %d agent nodes and this cluster has %d. This runtime cannot add nodes to a running "+
					"cluster: set AGENTS=%d in .env and rebuild the lab with 'labctl reset'.",
				next.Name, next.Agents, lab.Agents, next.Agents))
		}
	}

	all := append(slices.Clone(active), next)
	declared := f.Baseline + max(0, agents-1)*f.Agent + sharedCost(f, all) + ownCost(all)
	v.NeedMiB = declared
	if lab.UsedMiB > 0 {
		live := lab.UsedMiB + v.AddAgents*f.Agent + next.OwnMiB
		for _, item := range uniqueItems([]Demand{next}) {
			if !lab.Running[item] {
				live += f.cost(item)
			}
		}
		v.NeedMiB = max(declared, live)
	}
	v.LimitMiB = int(lab.Engine.MemGiB() * 1024 * usableShare)
	if v.NeedMiB > v.LimitMiB {
		v.Problems = append(v.Problems, memoryProblem(f, lab, active, next, v))
	}

	if next.CPUs > lab.Engine.CPUs {
		v.Warnings = append(v.Warnings, fmt.Sprintf(
			"%s runs best with %d CPUs and Docker has %d, so expect it to be slow. To add CPUs:\n  %s",
			next.Name, next.CPUs, lab.Engine.CPUs,
			lab.Host.ResizeHint(Need{CPUs: next.CPUs, MemGiB: math.Max(lab.Engine.MemGiB(), Floor.MemGiB)})))
	}
	for _, d := range active {
		if next.App != "" && d.App == next.App {
			v.Warnings = append(v.Warnings, fmt.Sprintf(
				"%s also uses %s, so their changes and grading can interfere. "+
					"To keep them apart, activate this one with --app <another app>.",
				d.Name, next.App))
		}
	}
	return v
}

// checkExclusive blocks next when it, or anything already active, must run
// alone.
func (v *Verdict) checkExclusive(active []Demand, next Demand) {
	if next.Exclusive && len(active) > 0 {
		v.Problems = append(v.Problems, fmt.Sprintf(
			"%s must run on its own. Bring down what is active first: labctl scenario down %s",
			next.Name, strings.Join(names(active), " ")))
	}
	for _, d := range active {
		if d.Exclusive {
			v.Problems = append(v.Problems, fmt.Sprintf(
				"%s is active and must run on its own. Bring it down first: labctl scenario down %s",
				d.Name, d.Name))
		}
	}
}

// memoryProblem explains a memory shortfall: what is using memory, what next
// adds, which activations to bring down to make room, and how big to make
// Docker instead.
func memoryProblem(f Footprints, lab Lab, active []Demand, next Demand, v Verdict) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Not enough memory for %s: the lab would need about %s GB and may use %s GB "+
		"(85%% of Docker's %s GB, keeping room for spikes).\n",
		next.Name, gbString(v.NeedMiB), gbString(v.LimitMiB), gbString(int(lab.Engine.MemBytes>>20)))

	target := math.Ceil(float64(v.NeedMiB) / usableShare / 1024)
	resize := Need{CPUs: max(lab.Engine.CPUs, Floor.CPUs), MemGiB: math.Max(target, Floor.MemGiB)}
	hint := lab.Host.ResizeHint(resize)
	if down := makeRoom(f, active, next, v.NeedMiB-v.LimitMiB); len(down) > 0 {
		fmt.Fprintf(&b, "Either bring something down:\n  labctl scenario down %s\n", strings.Join(down, " "))
		fmt.Fprintf(&b, "or give Docker %s GB:\n  %s", formatGiB(resize.MemGiB), hint)
	} else {
		fmt.Fprintf(&b, "Give Docker %s GB:\n  %s", formatGiB(resize.MemGiB), hint)
	}
	return b.String()
}

// makeRoom picks the active activations to bring down, largest saving first,
// until at least shortMiB is freed. It returns nil when bringing everything
// down would not be enough.
func makeRoom(f Footprints, active []Demand, next Demand, shortMiB int) []string {
	type saving struct {
		name string
		mib  int
	}
	savings := make([]saving, 0, len(active))
	for i, d := range active {
		others := append(slices.Clone(active[:i]), active[i+1:]...)
		others = append(others, next)
		freed := d.OwnMiB + sharedCost(f, append(slices.Clone(others), d)) - sharedCost(f, others)
		savings = append(savings, saving{name: d.Name, mib: freed})
	}
	slices.SortFunc(savings, func(a, b saving) int { return cmp.Compare(b.mib, a.mib) })

	picked := []string{}
	freed := 0
	for _, s := range savings {
		if freed >= shortMiB {
			break
		}
		picked = append(picked, s.name)
		freed += s.mib
	}
	if freed < shortMiB {
		return nil
	}
	return picked
}

// sharedCost is the footprint of every distinct item the demands use.
func sharedCost(f Footprints, demands []Demand) int {
	total := 0
	for _, item := range uniqueItems(demands) {
		total += f.cost(item)
	}
	return total
}

// ownCost is the memory the demands' own workloads use.
func ownCost(demands []Demand) int {
	total := 0
	for _, d := range demands {
		total += d.OwnMiB
	}
	return total
}

// uniqueItems lists each item the demands use once.
func uniqueItems(demands []Demand) []Item {
	seen := map[Item]bool{}
	items := []Item{}
	for _, d := range demands {
		for _, item := range d.Items {
			if !seen[item] {
				seen[item] = true
				items = append(items, item)
			}
		}
	}
	return items
}

// names lists the demands' names.
func names(demands []Demand) []string {
	out := make([]string, 0, len(demands))
	for _, d := range demands {
		out = append(out, d.Name)
	}
	return out
}

// gbString formats MiB as GB with one decimal.
func gbString(mib int) string {
	return formatGiB(math.Round(float64(mib)/1024*10) / 10)
}
