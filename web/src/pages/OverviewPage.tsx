import { Link } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import type { ComponentType, SVGProps } from 'react'
import { useChannels, useDomains, useIncidents, useMeta, useNotifications, useRules, useStats } from '../lib/api'
import { daysLabel, formatDateTime, relativeTime } from '../lib/format'
import { ExpiryHorizon } from '../components/ExpiryHorizon'
import { SetupChecklist } from '../components/SetupChecklist'
import {
  AlertIcon,
  ArrowUpRightIcon,
  ClockIcon,
  GlobeIcon,
  RadioIcon,
  RefreshIcon,
} from '../components/icons'
import { Banner, EmptyState, PageHeader, Panel, SecondaryButton, Spinner, StatusPill } from '../components/ui'

type IconComponent = ComponentType<SVGProps<SVGSVGElement>>

function Stat({
  icon: Icon,
  label,
  value,
  note,
}: {
  icon: IconComponent
  label: string
  value: string
  note: string
}) {
  return (
    <div className="flex items-start gap-3 px-4 py-4 sm:px-5">
      <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface-2 text-muted">
        <Icon className="size-4" />
      </span>
      <div className="min-w-0">
        <p className="text-[11px] font-medium text-muted">{label}</p>
        <p className="tnum mt-0.5 text-xl font-semibold tracking-[-0.03em] text-ink">{value}</p>
        <p className="mt-0.5 text-[10px] leading-4 text-faint">{note}</p>
      </div>
    </div>
  )
}

export function OverviewPage() {
  const queryClient = useQueryClient()
  const meta = useMeta()
  const stats = useStats()
  const domains = useDomains({ limit: 200 })
  const rules = useRules()
  const channels = useChannels()
  const incidents = useIncidents({ state: 'open', limit: 5 })
  const notifications = useNotifications({ limit: 6, exclude_archived: true })

  const refreshing =
    meta.isFetching ||
    stats.isFetching ||
    domains.isFetching ||
    rules.isFetching ||
    channels.isFetching ||
    incidents.isFetching

  const refresh = () => {
    void queryClient.invalidateQueries()
  }

  const items = domains.data?.items ?? []
  const active = items.filter((domain) => domain.state === 'active')
  const withExpiry = active
    .filter((domain) => domain.days_left !== undefined)
    .sort((a, b) => (a.days_left ?? 0) - (b.days_left ?? 0))
  const next = withExpiry[0]
  const recent = [...items]
    .filter((domain) => domain.last_checked_at)
    .sort((a, b) => new Date(b.last_checked_at ?? 0).getTime() - new Date(a.last_checked_at ?? 0).getTime())
    .slice(0, 6)
  const latestAlerts = (notifications.data?.items ?? []).slice(0, 5)
  const setupComplete =
    (meta.data?.auth_configured ?? false) &&
    (stats.data?.active ?? 0) > 0 &&
    (rules.data?.items.length ?? 0) > 0 &&
    (channels.data?.items.length ?? 0) > 0
  const showSetup = !setupComplete && !(stats.isPending || rules.isPending || channels.isPending)

  const error = stats.error ?? domains.error ?? rules.error ?? channels.error

  return (
    <div className="space-y-5">
      <PageHeader
        title="Overview"
        description="A calm view of every domain, alert offset, and delivery route before anything lapses."
        actions={
          <SecondaryButton onClick={refresh} title="Refresh all data">
            <RefreshIcon className={`size-3.5 ${refreshing ? 'animate-spin' : ''}`} />
            Refresh
          </SecondaryButton>
        }
      />

      {error ? (
        <Banner tone="error" title="Some dashboard data could not be loaded">
          {error instanceof Error ? error.message : 'The API did not answer.'}
        </Banner>
      ) : null}

      <div className="grid overflow-hidden rounded-[10px] border border-line bg-surface divide-y divide-line sm:grid-cols-2 sm:divide-x sm:divide-y-0 xl:grid-cols-4">
        <Stat
          icon={GlobeIcon}
          label="Domains under watch"
          value={String(stats.data?.active ?? 0)}
          note={`${stats.data?.total ?? 0} total, ${stats.data?.paused ?? 0} paused`}
        />
        <Stat
          icon={ClockIcon}
          label="Expiring in 30 days"
          value={String(stats.data?.expiring_30 ?? 0)}
          note={next ? `Next: ${next.name} · ${daysLabel(next.days_left)}` : 'No expiry recorded'}
        />
        <Stat
          icon={AlertIcon}
          label="Open incidents"
          value={String(stats.data?.open_incidents ?? 0)}
          note={`${stats.data?.unknown ?? 0} domains without a usable expiry`}
        />
        <Stat
          icon={RadioIcon}
          label="Delivery channels"
          value={String(channels.data?.items.length ?? 0)}
          note={`${rules.data?.items.length ?? 0} alert rules configured`}
        />
      </div>

      <div className={showSetup ? 'grid gap-5 xl:grid-cols-[minmax(0,1.55fr)_minmax(320px,1fr)]' : ''}>
        <ExpiryHorizon
          domains={withExpiry.map((domain) => ({
            id: domain.id,
            name: domain.name,
            daysRemaining: domain.days_left ?? 0,
          }))}
        />
        {showSetup ? (
          <SetupChecklist
            domainCount={stats.data?.active ?? 0}
            ruleCount={rules.data?.items.length ?? 0}
            channelCount={channels.data?.items.length ?? 0}
          />
        ) : null}
      </div>

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.45fr)_minmax(320px,1fr)]">
        <Panel
          title="Recent checks"
          description="The latest RDAP and WHOIS results across the watchlist."
          action={
            <Link
              to="/domains"
              className="inline-flex items-center gap-1 text-[11px] font-semibold text-accent hover:underline"
            >
              View domains
              <ArrowUpRightIcon className="size-3" />
            </Link>
          }
        >
          {domains.isPending ? (
            <Spinner label="Loading domains" />
          ) : recent.length === 0 ? (
            <div className="px-4 py-4 sm:px-5">
              <EmptyState
                compact
                icon={GlobeIcon}
                title="No checks have run yet"
                description="Add a domain and IWMMD will queue the first RDAP check immediately."
                action={
                  <Link
                    to="/domains"
                    className="inline-flex h-9 items-center rounded-md bg-accent px-3 text-xs font-semibold text-canvas hover:bg-accent/90"
                  >
                    Add a domain
                  </Link>
                }
              />
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full min-w-[36rem] border-collapse text-left">
                <thead>
                  <tr className="border-b border-line">
                    {['Domain', 'Expiry', 'Last check', 'Result'].map((heading) => (
                      <th
                        key={heading}
                        scope="col"
                        className="px-4 py-2.5 text-[10px] font-semibold text-faint last:text-right sm:px-5"
                      >
                        {heading}
                      </th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {recent.map((domain) => (
                    <tr key={domain.id} className="transition-colors hover:bg-surface-2/45">
                      <td className="px-4 py-3 sm:px-5">
                        <Link to="/domains" className="text-xs font-semibold text-ink hover:text-accent">
                          {domain.name}
                        </Link>
                        <p className="mt-0.5 text-[10px] text-faint">
                          {domain.registrar || 'Registrar unknown'}
                        </p>
                      </td>
                      <td className="px-4 py-3 text-[11px] text-muted sm:px-5">
                        {domain.expires_at ? daysLabel(domain.days_left) : 'Unknown'}
                      </td>
                      <td className="px-4 py-3 text-[11px] text-muted sm:px-5" title={formatDateTime(domain.last_checked_at)}>
                        {relativeTime(domain.last_checked_at)}
                      </td>
                      <td className="px-4 py-3 text-right sm:px-5">
                        <StatusPill
                          tone={
                            domain.last_check_status === 'ok'
                              ? 'ok'
                              : domain.last_check_status
                                ? 'error'
                                : 'neutral'
                          }
                        >
                          {domain.last_check_status || 'Not checked'}
                        </StatusPill>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Panel>

        <Panel
          title="Recent notifications"
          description="Expiry and system deliveries recorded in the outbox."
          action={
            <Link
              to="/incidents"
              className="inline-flex items-center gap-1 text-[11px] font-semibold text-accent hover:underline"
            >
              Open incidents
              <ArrowUpRightIcon className="size-3" />
            </Link>
          }
        >
          {notifications.isPending ? (
            <Spinner label="Loading notifications" />
          ) : latestAlerts.length === 0 ? (
            <div className="px-4 py-4 sm:px-5">
              <EmptyState
                compact
                icon={AlertIcon}
                title="No notifications yet"
                description="Connect a channel and add an alert rule. Delivery history appears here."
              />
            </div>
          ) : (
            <ul className="divide-y divide-line">
              {latestAlerts.map((item) => (
                <li key={item.id} className="px-4 py-3 sm:px-5">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <p className="truncate text-xs font-semibold text-ink">
                        {item.domain_name || 'System message'}
                      </p>
                      <p className="mt-0.5 text-[10px] leading-4 text-muted">
                        {item.channel_name || 'Unknown channel'} · {formatDateTime(item.scheduled_at)}
                      </p>
                    </div>
                    <StatusPill
                      tone={
                        item.state === 'sent'
                          ? 'ok'
                          : item.state === 'failed' || item.state === 'dead'
                            ? 'error'
                            : item.state === 'pending' || item.state === 'sending'
                              ? 'warn'
                              : 'neutral'
                      }
                      dot={false}
                    >
                      {item.state}
                    </StatusPill>
                  </div>
                  {item.last_error ? (
                    <p className="mt-1 line-clamp-2 text-[10px] leading-4 text-danger">{item.last_error}</p>
                  ) : null}
                </li>
              ))}
            </ul>
          )}
        </Panel>
      </div>

      {incidents.data?.items.length ? (
        <Panel title="Open incidents" description="Check failures and unparseable expiry answers.">
          <ul className="divide-y divide-line">
            {incidents.data.items.map((incident) => (
              <li key={incident.id} className="flex items-center justify-between gap-3 px-4 py-3 sm:px-5">
                <div>
                  <p className="text-xs font-semibold text-ink">{incident.kind.replaceAll('_', ' ')}</p>
                  <p className="mt-0.5 text-[10px] text-faint">
                    Opened {relativeTime(incident.opened_at)} · last seen {relativeTime(incident.last_seen_at)}
                  </p>
                </div>
                <StatusPill tone="warn">open</StatusPill>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}
    </div>
  )
}
