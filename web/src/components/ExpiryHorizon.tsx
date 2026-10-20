import { useEffect, useMemo, useRef, useState } from 'react'
import { daysLabel } from '../lib/format'
import { ClockIcon } from './icons'
import { Panel, StatusPill } from './ui'

export type HorizonDomain = {
  id: string
  name: string
  daysRemaining: number
}

type HorizonRange = 90 | 365

const ranges: Array<{ value: HorizonRange; label: string; title: string }> = [
  { value: 90, label: '90d', title: 'Show the next 90 days' },
  { value: 365, label: '1y', title: 'Show the next year' },
]

const labelWidth = 112
const labelGap = 10
const laneHeight = 26
const baselineOffset = 14
const minChartHeight = 208

function ticksFor(range: HorizonRange) {
  const days = range === 90 ? [90, 60, 30, 14, 7, 0] : [365, 270, 180, 90, 30, 0]
  return days.map((value) => ({
    label: value === 0 ? '0d' : value === 365 ? '1y' : `${value}d`,
    position: ((range - value) / range) * 100,
  }))
}

function markerTone(days: number) {
  if (days <= 7) return 'bg-danger'
  if (days <= 30) return 'bg-signal'
  return 'bg-accent'
}

type PlottedDomain = HorizonDomain & {
  left: number
  lane: number
}

export function ExpiryHorizon({ domains = [] }: { domains?: HorizonDomain[] }) {
  const [manualRange, setManualRange] = useState<HorizonRange | null>(null)
  const chartRef = useRef<HTMLDivElement>(null)
  const [plotWidth, setPlotWidth] = useState(0)

  const furthest = domains.reduce((max, domain) => Math.max(max, domain.daysRemaining), 0)
  const range = manualRange ?? (furthest > 90 ? 365 : 90)
  const ticks = useMemo(() => ticksFor(range), [range])
  const beyond = domains.filter((domain) => domain.daysRemaining > range).length
  const rangeLabel = range === 90 ? '90-day' : '1-year'

  useEffect(() => {
    const node = chartRef.current
    if (!node) {
      setPlotWidth(0)
      return
    }
    const update = () => setPlotWidth(node.clientWidth)
    update()
    const observer = new ResizeObserver(update)
    observer.observe(node)
    return () => observer.disconnect()
  }, [domains.length])

  const plotted = useMemo(() => {
    const items: PlottedDomain[] = domains
      .map((domain) => {
        const clampedDays = Math.min(range, Math.max(0, domain.daysRemaining))
        return {
          ...domain,
          left: ((range - clampedDays) / range) * 100,
          lane: 0,
        }
      })
      .sort((a, b) => a.left - b.left)

    if (plotWidth <= 0) return items

    const laneEnds: number[] = []
    for (const item of items) {
      const center = (item.left / 100) * plotWidth
      const labelLeft =
        item.left < 15
          ? center
          : item.left > 85
            ? center - labelWidth
            : center - labelWidth / 2
      const labelRight = labelLeft + labelWidth

      let lane = 0
      while (lane < laneEnds.length && labelLeft < laneEnds[lane] + labelGap) {
        lane += 1
      }
      if (lane === laneEnds.length) laneEnds.push(labelRight)
      else laneEnds[lane] = labelRight
      item.lane = lane
    }
    return items
  }, [domains, plotWidth, range])

  const laneCount = plotted.reduce((max, item) => Math.max(max, item.lane + 1), 1)
  const chartHeight = Math.max(minChartHeight, 104 + (laneCount - 1) * laneHeight)

  return (
    <Panel
      className="flex h-full flex-col"
      bodyClassName="flex flex-1 flex-col"
      title="Expiry horizon"
      description={`Every active domain on a ${rangeLabel} scale.`}
      action={
        domains.length > 0 ? (
          <div
            className="flex items-center gap-1 rounded-md border border-line bg-surface p-0.5"
            role="group"
            aria-label="Expiry horizon range"
          >
            {ranges.map((option) => (
              <button
                key={option.value}
                type="button"
                title={option.title}
                aria-pressed={range === option.value}
                onClick={() => setManualRange(option.value)}
                className={`rounded px-2 py-1 font-mono text-[10px] font-semibold transition-colors ${
                  range === option.value
                    ? 'bg-accent-soft text-accent'
                    : 'text-faint hover:bg-surface-2 hover:text-ink'
                }`}
              >
                {option.label}
              </button>
            ))}
          </div>
        ) : undefined
      }
    >
      <div className="flex flex-1 flex-col px-4 pb-4 pt-5 sm:px-5">
        <div
          className="relative flex-1 overflow-hidden rounded-md border border-line bg-surface-2/55"
          style={{ minHeight: chartHeight }}
        >
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
            <>
              <div ref={chartRef} className="absolute inset-x-6 bottom-9 top-4">
                {ticks.map((tick) => (
                  <span
                    key={tick.label}
                    className="absolute inset-y-0 w-px bg-line/70"
                    style={{ left: `${tick.position}%` }}
                    aria-hidden="true"
                  />
                ))}
                <span className="absolute inset-x-0 bottom-0 h-px bg-line-strong" aria-hidden="true" />
                {plotted.map((domain) => {
                  const dotBottom = baselineOffset + domain.lane * laneHeight
                  const labelBottom = dotBottom + 12
                  return (
                    <div
                      key={domain.id}
                      className="absolute bottom-0"
                      style={{ left: `${domain.left}%` }}
                      aria-label={`${domain.name}, ${daysLabel(domain.daysRemaining)}`}
                    >
                      <span
                        className="absolute bottom-0 left-0 w-px bg-line-strong"
                        style={{ height: `${dotBottom + 4}px` }}
                        aria-hidden="true"
                      />
                      <span
                        className={`absolute left-[-4px] size-2 rounded-full ring-4 ring-surface-2 ${markerTone(domain.daysRemaining)}`}
                        style={{ bottom: `${dotBottom}px` }}
                        aria-hidden="true"
                      />
                      <span
                        title={`${domain.name} · ${daysLabel(domain.daysRemaining)}`}
                        className="absolute max-w-28 truncate rounded border border-line bg-surface px-1.5 py-0.5 font-mono text-[10px] text-ink"
                        style={{
                          bottom: `${labelBottom}px`,
                          transform:
                            domain.left < 15
                              ? 'translateX(0)'
                              : domain.left > 85
                                ? 'translateX(-100%)'
                                : 'translateX(-50%)',
                        }}
                      >
                        {domain.name}
                      </span>
                    </div>
                  )
                })}
              </div>
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
            </>
          )}
        </div>

        <div className="mt-3 flex flex-wrap items-center justify-between gap-2">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] font-medium text-faint">
            <span className="inline-flex items-center gap-1.5">
              <span className="size-1.5 rounded-full bg-accent" /> 31d+
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="size-1.5 rounded-full bg-signal" /> 8-30d
            </span>
            <span className="inline-flex items-center gap-1.5">
              <span className="size-1.5 rounded-full bg-danger" /> 7d or less
            </span>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {beyond > 0 ? (
              <StatusPill tone="warn" dot={false}>
                {beyond} beyond {range === 90 ? '90d' : '1y'}
              </StatusPill>
            ) : null}
            <StatusPill tone="neutral" dot={false}>
              {domains.length} plotted
            </StatusPill>
          </div>
        </div>
      </div>
    </Panel>
  )
}
