import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ClusterDownBanner } from './App'

describe('ClusterDownBanner', () => {
  it('names the cluster, the reason and the recovery command', () => {
    render(<ClusterDownBanner info={{
      context: 'k3d-snowops', server: '', k8sVersion: 'unknown', nodeCount: 0,
      connected: false, error: 'net/http: TLS handshake timeout',
    }} />)
    const alert = screen.getByRole('alert')
    expect(alert).toHaveTextContent('k3d-snowops is unreachable')
    expect(alert).toHaveTextContent('TLS handshake timeout')
    expect(alert).toHaveTextContent('labctl init')
  })

  it('tells a new user to create the lab when there is no context', () => {
    render(<ClusterDownBanner info={{
      context: '', server: '', k8sVersion: '', nodeCount: 0, connected: false,
    }} />)
    expect(screen.getByRole('alert')).toHaveTextContent('No cluster yet')
  })
})
