import type { Capacity } from '../types'

/** Share of the usable memory in use at which the card warns that a new scenario may be refused. */
const NEAR_LIMIT = 0.9

const RESOURCES_DOC = 'https://labs.snowops.net/docs/resources'

function gb(mib: number) {
  return (mib / 1024).toFixed(1)
}

/**
 * DockerCard shows the Docker engine's size and how much of the memory the lab
 * may plan to use is taken, so a learner sees why a scenario would be refused
 * before starting it.
 */
export function DockerCard({ capacity }: { capacity: Capacity }) {
  const share = capacity.usableMiB > 0 ? capacity.usedMiB / capacity.usableMiB : 0
  const pct = Math.min(100, Math.round(share * 100))
  const nearLimit = share >= NEAR_LIMIT

  return (
    <div className="card">
      <div className="card-header">
        <span className="card-title">Docker</span>
      </div>
      <div className="card-body">
        <div className="row"><span className="label">CPUs</span><span className="value tnum">{capacity.cpus}</span></div>
        <div className="row"><span className="label">Memory</span><span className="value tnum">{gb(capacity.memoryMiB)} GB</span></div>
        <div className="row"><span className="label">Lab may use</span><span className="value tnum">{gb(capacity.usableMiB)} GB</span></div>
        <div className="row"><span className="label">In use</span><span className="value tnum">{gb(capacity.usedMiB)} GB</span></div>
        <div
          className="progress"
          role="meter"
          aria-label="Lab memory in use"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={pct}
        >
          <div className="progress-track">
            <div className="progress-fill" style={{ width: `${pct}%` }} />
          </div>
          <span className="progress-pct">{pct}%</span>
        </div>
        {nearLimit && (
          <div className="empty-hint">
            Close to the limit: a new scenario may be refused. Bring one down, or give Docker
            more memory (<a href={RESOURCES_DOC} target="_blank" rel="noopener noreferrer">how</a>).
          </div>
        )}
      </div>
    </div>
  )
}
