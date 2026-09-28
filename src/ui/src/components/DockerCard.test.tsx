// SPDX-License-Identifier: Apache-2.0

import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DockerCard } from './DockerCard'

describe('DockerCard', () => {
  it('shows the engine size and the share of usable memory in use', () => {
    render(<DockerCard capacity={{ cpus: 2, memoryMiB: 3905, usedMiB: 1660, usableMiB: 3319 }} />)
    expect(screen.getByText('3.8 GB')).toBeInTheDocument()
    expect(screen.getByText('3.2 GB')).toBeInTheDocument()
    expect(screen.getByText('1.6 GB')).toBeInTheDocument()
    expect(screen.getByRole('meter', { name: 'Lab memory in use' })).toHaveAttribute('aria-valuenow', '50')
    expect(screen.queryByText(/Close to the limit/)).not.toBeInTheDocument()
  })

  it('warns when the lab is close to the limit', () => {
    render(<DockerCard capacity={{ cpus: 2, memoryMiB: 3905, usedMiB: 3200, usableMiB: 3319 }} />)
    expect(screen.getByText(/Close to the limit/)).toBeInTheDocument()
  })

  it('caps the meter at 100% when use exceeds the plan', () => {
    render(<DockerCard capacity={{ cpus: 2, memoryMiB: 3905, usedMiB: 3600, usableMiB: 3319 }} />)
    expect(screen.getByRole('meter', { name: 'Lab memory in use' })).toHaveAttribute('aria-valuenow', '100')
  })
})
