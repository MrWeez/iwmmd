import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  api,
  useDomains,
  useIncidents,
  useMe,
  useNotifications,
  type Notification,
} from '../lib/api'
import { formatDateTime, relativeTime, titleCase } from '../lib/format'
import { AlertIcon, RefreshIcon } from '../components/icons'
import {
  Banner,
  EmptyState,
  PageHeader,
  Panel,
  SecondaryButton,
  Select,
  Spinner,
  StatusPill,
} from '../components/ui'

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return 'The request could not be completed.'
}

function stateTone(state: string): 'ok' | 'warn' | 'error' | 'neutral' {
  if (state === 'sent' || state === 'resolved') return 'ok'
  if (state === 'failed' || state === 'dead') return 'error'
  if (state === 'pending' || state === 'sending' || state === 'open') return 'warn'
  return 'neutral'
}

export function IncidentsPage() {
  const client = useQueryClient()
  const me = useMe()
  const isAdmin = me.data?.user?.role === 'admin'
  const [state, setState] = useState('all')
  const [retryTarget, setRetryTarget] = useState<Notification | null>(null)

  const incidents = useIncidents({ limit: 100 })
  const notifications = useNotifications({
    state: state === 'all' ? undefined : state,
    limit: 100,
    exclude_archived: true,
  })
  const domains = useDomains({ limit: 200 })

  const domainNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const domain of domains.data?.items ?? []) map.set(domain.id, domain.name)
    return map
  }, [domains.data])

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['incidents'] })
    void client.invalidateQueries({ queryKey: ['notifications'] })
    void client.invalidateQueries({ queryKey: ['stats'] })
  }

  const retry = useMutation({
    mutationFn: (notification: Notification) => api.retryNotification(notification.id),
    onSuccess: () => {
      setRetryTarget(null)
      refresh()
    },
  })

  const openIncidents = (incidents.data?.items ?? []).filter((incident) => incident.state === 'open')
  const items = notifications.data?.items ?? []
  const failed = items.filter((item) => item.state === 'failed').length

  return (
    <div className="space-y-5">
      <PageHeader
        title="Incidents"
        description="Check failures, unparseable expiry answers, and delivery problems that need attention."
        actions={
          <SecondaryButton onClick={refresh} title="Refresh incident data">
            <RefreshIcon className="size-3.5" />
            Refresh
          </SecondaryButton>
        }
      />

      {(incidents.error ?? notifications.error) ? (
        <Banner tone="error" title="Some incident data could not be loaded">
          {errorMessage(incidents.error ?? notifications.error)}
        </Banner>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-3">
        <Summary label="Open incidents" value={openIncidents.length} tone={openIncidents.length ? 'warn' : 'ok'} />
        <Summary label="Failed deliveries" value={failed} tone={failed ? 'error' : 'ok'} />
        <Summary
          label="Queued notifications"
          value={items.filter((item) => item.state === 'pending' || item.state === 'sending').length}
          tone="neutral"
        />
      </div>

      <Panel
        title="System incidents"
        description="Opened by check failures and unusable expiry answers."
        action={
          <StatusPill tone={openIncidents.length ? 'warn' : 'ok'} dot={false}>
            {openIncidents.length} open
          </StatusPill>
        }
      >
        {incidents.isPending ? (
          <Spinner label="Loading incidents" />
        ) : (incidents.data?.items.length ?? 0) === 0 ? (
          <div className="px-4 py-5 sm:px-5">
            <EmptyState
              compact
              icon={AlertIcon}
              title="No incidents recorded"
              description="A domain with a failing check or an unknown expiry will appear here."
            />
          </div>
        ) : (
          <ul className="divide-y divide-line">
            {incidents.data?.items.map((incident) => (
              <li key={incident.id} className="px-4 py-3.5 sm:px-5">
                <div className="flex flex-wrap items-start justify-between gap-2">
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <p className="text-xs font-semibold text-ink">{titleCase(incident.kind)}</p>
                      <StatusPill tone={stateTone(incident.state)}>{incident.state}</StatusPill>
                    </div>
                    <p className="mt-0.5 text-[10px] text-faint">
                      {domainNames.get(incident.domain_id) ?? 'Unknown domain'} · opened {relativeTime(incident.opened_at)}
                      {incident.resolved_at ? ` · resolved ${relativeTime(incident.resolved_at)}` : ''}
                    </p>
                  </div>
                  <p className="text-[10px] text-faint">last seen {formatDateTime(incident.last_seen_at)}</p>
                </div>
                {incident.details && incident.details !== '{}' ? (
                  <p className="mt-2 line-clamp-2 rounded border border-line bg-surface-2/40 px-2 py-1.5 font-mono text-[10px] text-muted">
                    {incident.details}
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </Panel>

      <Panel
        title="Delivery outbox"
        description="Current and system deliveries. Archived domain history stays in the domain detail."
        action={
          <Select value={state} onChange={(event) => setState(event.target.value)} className="w-36" aria-label="Filter notification state">
            <option value="all">All states</option>
            <option value="pending">Pending</option>
            <option value="sending">Sending</option>
            <option value="sent">Sent</option>
            <option value="failed">Failed</option>
            <option value="suppressed">Suppressed</option>
            <option value="canceled">Canceled</option>
          </Select>
        }
      >
        {notifications.isPending ? (
          <Spinner label="Loading notifications" />
        ) : items.length === 0 ? (
          <div className="px-4 py-5 sm:px-5">
            <EmptyState
              compact
              icon={AlertIcon}
              title="No notifications in this state"
              description="Alert rules generate notifications once a domain has a usable expiry date and a channel."
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[58rem] border-collapse text-left">
              <thead>
                <tr className="border-b border-line">
                  {['Domain', 'Kind', 'Channel', 'State', 'Scheduled', 'Attempts', ''].map((heading, index) => (
                    <th
                      key={`${heading}-${index}`}
                      scope="col"
                      className="px-4 py-2.5 text-[10px] font-semibold text-faint last:text-right sm:px-5"
                    >
                      {heading}
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {items.map((item) => (
                  <tr key={item.id} className="align-top transition-colors hover:bg-surface-2/45">
                    <td className="px-4 py-3 sm:px-5">
                      <p className="text-xs font-semibold text-ink">
                        {item.domain_name || domainNames.get(item.domain_id ?? '') || 'System'}
                      </p>
                      {item.expiry_date ? (
                        <p className="mt-0.5 text-[10px] text-faint">Expiry {item.expiry_date}</p>
                      ) : null}
                    </td>
                    <td className="px-4 py-3 text-[11px] text-muted sm:px-5">{titleCase(item.kind)}</td>
                    <td className="px-4 py-3 text-[11px] text-muted sm:px-5">{item.channel_name || 'Unknown'}</td>
                    <td className="px-4 py-3 sm:px-5">
                      <StatusPill tone={stateTone(item.state)}>{item.state}</StatusPill>
                    </td>
                    <td className="px-4 py-3 text-[10px] text-muted sm:px-5">
                      {formatDateTime(item.scheduled_at)}
                      {item.sent_at ? (
                        <p className="mt-0.5 text-faint">Sent {relativeTime(item.sent_at)}</p>
                      ) : null}
                    </td>
                    <td className="px-4 py-3 text-[10px] text-muted sm:px-5">{item.attempts}</td>
                    <td className="px-4 py-3 text-right sm:px-5">
                      {isAdmin && ['failed', 'suppressed', 'canceled'].includes(item.state) ? (
                        <SecondaryButton onClick={() => setRetryTarget(item)}>Retry</SecondaryButton>
                      ) : (
                        <span className="text-[10px] text-faint">-</span>
                      )}
                      {item.last_error ? (
                        <p className="mt-1 max-w-56 text-left text-[10px] leading-4 text-danger">{item.last_error}</p>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {retryTarget ? (
        <div className="fixed inset-0 z-50 grid place-items-center bg-canvas/80 px-4 backdrop-blur-[2px]">
          <div className="w-full max-w-md rounded-[10px] border border-line bg-surface p-5 shadow-xl">
            <h2 className="text-sm font-semibold text-ink">Retry notification?</h2>
            <p className="mt-2 text-xs leading-5 text-muted">
              The outbox entry for {retryTarget.domain_name || 'this domain'} will be reset to pending with zero
              attempts and delivered on the next dispatcher pass.
            </p>
            {retry.error ? (
              <div className="mt-3">
                <Banner tone="error" title="Retry failed">
                  {errorMessage(retry.error)}
                </Banner>
              </div>
            ) : null}
            <div className="mt-5 flex justify-end gap-2">
              <SecondaryButton onClick={() => setRetryTarget(null)}>Cancel</SecondaryButton>
              <SecondaryButton onClick={() => retry.mutate(retryTarget)} disabled={retry.isPending}>
                {retry.isPending ? 'Queueing…' : 'Retry delivery'}
              </SecondaryButton>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function Summary({
  label,
  value,
  tone,
}: {
  label: string
  value: number
  tone: 'ok' | 'warn' | 'error' | 'neutral'
}) {
  return (
    <div className="rounded-[10px] border border-line bg-surface px-4 py-3.5">
      <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-faint">{label}</p>
      <div className="mt-1 flex items-end justify-between gap-2">
        <p className="tnum text-2xl font-semibold tracking-[-0.04em] text-ink">{value}</p>
        <StatusPill tone={tone} dot={false}>
          {tone === 'ok' ? 'clear' : tone === 'error' ? 'attention' : tone}
        </StatusPill>
      </div>
    </div>
  )
}
