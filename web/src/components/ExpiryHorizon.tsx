import { ClockIcon } from './icons'
import { Panel, StatusPill } from './ui'

export type HorizonDomain = {
  id: string
  name: string
  daysRemaining: number
}

const ticks = [
  { label: '90d', position: 0 },
  { label: '60d', position: 33.333 },
  { label: '30d', position: 66.666 },
  { label: '14d', position: 84.444 },
  { label: '7d', position: 92.222 },
  { label: '0d', position: 100 },
]

function markerTone(days: number) {
  if (days <= 7) return 'bg-danger'
  if (days <= 30) return 'bg-signal'
  return 'bg-accent'
}

export function ExpiryHorizon({ domains = [] }: { domains?: HorizonDomain[] }) {
  return (
    <Panel
      className="flex h-full flex-col"
      bodyClassName="flex flex-1 flex-col"
      title="Expiry horizon"
      description="Every domain, placed by days remaining."
      action={
        <div className="flex items-center gap-3 text-[10px] font-medium text-faint">
          <span className="inline-flex items-center gap-1.5">
            <span className="size-1.5 rounded-full bg-accent" /> 90-31d
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-1.5 rounded-full bg-signal" /> 30-8d
          </span>
          <span className="inline-flex items-center gap-1.5">
            <span className="size-1.5 rounded-full bg-danger" /> 7d or less
          </span>
        </div>
      }
    >
      <div className="flex flex-1 flex-col px-4 pb-4 pt-5 sm:px-5">
        <div className="relative min-h-52 flex-1 overflow-hidden rounded-md border border-line bg-surface-2/55">
          <div className="horizon-grid absolute inset-x-6 bottom-9 top-8 opacity-70" aria-hidden="true" />
          <div className="absolute inset-x-6 bottom-9 h-px bg-line-strong" aria-hidden="true" />

          {domains.length === 0 ? (
            <div className="absolute inset-0 flex flex-col items-center justify-center px-6 text-center">
              <span className="grid size-9 place-items-center rounded-full border border-line bg-surface text-muted">
                <ClockIcon className="size-4" />
              </span>
              <p className="mt-2.5 text-sm font-semibold text-ink">No domains under watch</p>
              <p className="mt-1 max-w-sm text-xs leading-5 text-muted">
                Add a domain and it will appear here, moving toward expiry as the clock runs down.
              </p>
            </div>
          ) : (
            <div className="absolute inset-x-6 bottom-9 top-8">
              {domains.map((domain) => {
                const clamped = Math.min(90, Math.max(0, domain.daysRemaining))
                const left = ((90 - clamped) / 90) * 100
                return (
                  <div
                    key={domain.id}
                    className="absolute bottom-0 -translate-x-1/2"
                    style={{ left: `${left}%` }}
                  >
                    <div className="mb-1.5 flex flex-col items-center gap-1">
                      <span
                        className="max-w-28 truncate rounded border border-line bg-surface px-1.5 py-0.5 font-mono text-[10px] text-ink"
                        style={{
                          transform: left < 15 ? 'translateX(50%)' : left > 85 ? 'translateX(-50%)' : undefined,
                        }}
                      >
                        {domain.name}
                      </span>
                      <span className={`size-2 rounded-full ring-4 ring-surface-2 ${markerTone(domain.daysRemaining)}`} />
                    </div>
                    <div className="mx-auto h-4 w-px bg-line-strong" />
                  </div>
                )
              })}
            </div>
          )}

          <div className="absolute inset-x-6 bottom-3">
            {ticks.map((tick) => (
              <span
                key={tick.label}
                className="tnum absolute font-mono text-[10px] text-faint"
                style={{
                  left: `${tick.position}%`,
                  transform:
                    tick.position === 0
                      ? 'none'
                      : tick.position === 100
                        ? 'translateX(-100%)'
                        : 'translateX(-50%)',
                }}
              >
                {tick.label}
              </span>
            ))}
          </div>
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <p className="text-[11px] leading-5 text-muted">
            Markers shift from teal to amber at 30 days and to red at 7 days.
          </p>
          <StatusPill tone="neutral" dot={false}>
            {domains.length} plotted
          </StatusPill>
        </div>
      </div>
    </Panel>
  )
}
