import { describe, it, expect } from 'vitest'
import { requirementLabels } from './requirements'

describe('requirementLabels', () => {
  it.each([
    { name: 'nothing declared', req: undefined, expected: [] },
    { name: 'one agent is the default', req: { agents: 1 }, expected: [] },
    { name: 'a drill', req: { agents: 2, exclusive: true }, expected: ['Needs 2 agent nodes', 'Runs alone'] },
    { name: 'cpus but not memory', req: { cpus: 4, memory: '300Mi' }, expected: ['Best with 4 CPUs'] },
  ])('$name', ({ req, expected }) => {
    expect(requirementLabels(req)).toEqual(expected)
  })
})
