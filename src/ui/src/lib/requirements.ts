import type { ScenarioRequirements } from '../types'

// requirementLabels turns a scenario's requirements into short badges. Memory
// is left out: it is only the scenario's own share, not what it costs the lab.
export function requirementLabels(r: ScenarioRequirements | undefined): string[] {
  const labels: string[] = []
  if (r?.agents && r.agents > 1) labels.push(`Needs ${r.agents} agent nodes`)
  if (r?.cpus) labels.push(`Best with ${r.cpus} CPUs`)
  if (r?.exclusive) labels.push('Runs alone')
  return labels
}
