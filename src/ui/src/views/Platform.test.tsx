// SPDX-License-Identifier: Apache-2.0
//
// The Platform view must not guess while the cluster is unreachable: every
// component would otherwise read "Not Installed" and invite a pointless
// install. The api boundary is mocked, as in Traffic.test.tsx.
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { Platform } from './Platform'

vi.mock('../api/client', () => ({
  api: {
    getPlatform: vi.fn(),
    getDashboards: vi.fn(),
    getComponent: vi.fn(),
    componentUp: vi.fn(),
    componentDown: vi.fn(),
  },
}))

import { api } from '../api/client'
const mockApi = api as unknown as { getPlatform: Mock; getDashboards: Mock }

function renderPlatform(clusterDown: boolean) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={client}>
      <Platform notify={vi.fn()} requestConfirm={vi.fn()} clusterDown={clusterDown} />
    </QueryClientProvider>,
  )
}

describe('Platform view', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockApi.getDashboards.mockResolvedValue([])
    mockApi.getPlatform.mockResolvedValue({
      ingress: [{ name: 'traefik', category: 'ingress', installed: false, exclusive: true }],
    })
  })

  it('shows install state and an Install action when the cluster answers', async () => {
    renderPlatform(false)
    expect(await screen.findByText('Not Installed')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Install' })).toBeEnabled()
  })

  it('shows Unknown and holds actions while the cluster is unreachable', async () => {
    renderPlatform(true)
    expect(await screen.findByText('Unknown')).toBeInTheDocument()
    expect(screen.queryByText('Not Installed')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Install' })).toBeDisabled()
  })
})
