import { useMemo, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  api,
  useChannels,
  useDomains,
  useMe,
  useRules,
  useSettings,
  type Rule,
} from '../lib/api'
import { offsetLabel, titleCase } from '../lib/format'
import { BellIcon, PencilIcon, PlusIcon, TrashIcon } from '../components/icons'
import {
  Banner,
  Checkbox,
  DangerButton,
  Dialog,
  EmptyState,
  FormField,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  Select,
  Spinner,
  StatusPill,
  TextInput,
} from '../components/ui'

const presets = [
  { label: '2 months before', value: 2, unit: 'months', direction: 'before' },
  { label: '30 days before', value: 30, unit: 'days', direction: 'before' },
  { label: '14 days before', value: 14, unit: 'days', direction: 'before' },
  { label: '7 days before', value: 7, unit: 'days', direction: 'before' },
  { label: '3 days before', value: 3, unit: 'days', direction: 'before' },
  { label: '1 day before', value: 1, unit: 'days', direction: 'before' },
  { label: 'On expiry day', value: 0, unit: 'days', direction: 'before' },
  { label: '1 day after', value: 1, unit: 'days', direction: 'after' },
  { label: '7 days after', value: 7, unit: 'days', direction: 'after' },
  { label: '30 days after', value: 30, unit: 'days', direction: 'after' },
] as const

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return 'The request could not be completed.'
}

function AlertTimeline({ count }: { count: number }) {
  const labels = [
    { label: '90 days', position: 0, align: 'start' },
    { label: '30 days', position: 28, align: 'middle' },
    { label: '7 days', position: 52, align: 'middle' },
    { label: 'expiry', position: 65, align: 'middle', strong: true },
    { label: 'after', position: 82, align: 'middle' },
  ]

  return (
    <div className="px-4 pb-5 pt-6 sm:px-5">
      <div className="relative h-36 overflow-hidden rounded-md border border-line bg-surface-2/55">
        <div className="absolute inset-x-6 top-1/2 -translate-y-1/2" aria-hidden="true">
          <div className="flex items-center">
            <div className="h-px bg-accent/70" style={{ width: '65%' }} />
            <div className="h-8 w-px bg-ink/70" />
            <div className="h-px flex-1 border-t border-dashed border-line-strong" />
          </div>
        </div>
        <div className="absolute inset-x-6 top-[calc(50%-2.6rem)]">
          {labels.map((item) => (
            <span
              key={item.label}
              className={`absolute font-mono text-[10px] ${
                item.strong ? 'font-semibold text-ink' : 'text-faint'
              }`}
              style={{
                left: `${item.position}%`,
                transform:
                  item.align === 'start'
                    ? 'none'
                    : item.align === 'middle'
                      ? 'translateX(-50%)'
                      : 'translateX(-100%)',
              }}
            >
              {item.label}
            </span>
          ))}
        </div>
        <div className="absolute inset-x-6 top-[calc(50%+1.1rem)] flex justify-between">
          <span className="text-[10px] font-medium text-accent">Before expiry</span>
          <span className="text-[10px] font-medium text-muted">After expiry</span>
        </div>
        <div className="absolute bottom-3 right-4">
          <StatusPill tone={count ? 'ok' : 'neutral'} dot={false}>
            {count} {count === 1 ? 'rule' : 'rules'}
          </StatusPill>
        </div>
      </div>
      <p className="mt-3 text-[11px] leading-5 text-muted">
        Offsets are calendar-aware. A rule can fire several times before expiry and continue after it, each at its own send time.
      </p>
    </div>
  )
}

const mechanics = [
  {
    title: 'Calendar months stay honest',
    detail: 'A one-month offset from 31 January lands on the last valid day of February, not a fixed 30 days.',
  },
  {
    title: 'Expiry date is the identity',
    detail: 'A renewal cancels pending alerts for the old date. Re-checking the same date never duplicates a send.',
  },
  {
    title: 'Mute holds, then catches up',
    detail: 'Muted delivery waits. When the mute ends, only the latest due offset is sent, not the whole backlog.',
  },
]

export function AlertsPage() {
  const client = useQueryClient()
  const me = useMe()
  const role = me.data?.user?.role ?? 'viewer'
  const isAdmin = role === 'admin'
  const canWrite = role === 'member' || role === 'admin'

  const rules = useRules()
  const channels = useChannels()
  const domains = useDomains({ limit: 200 })
  const settings = useSettings()

  const [editorRule, setEditorRule] = useState<Rule | 'new' | null>(null)
  const [archiveTarget, setArchiveTarget] = useState<Rule | null>(null)

  const channelNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const channel of channels.data?.items ?? []) map.set(channel.id, channel.name)
    return map
  }, [channels.data])

  const domainNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const domain of domains.data?.items ?? []) map.set(domain.id, domain.name)
    return map
  }, [domains.data])

  const items = rules.data?.items ?? []
  const globalCount = items.filter((rule) => rule.scope === 'global').length
  const missingChannels = (channels.data?.items.length ?? 0) === 0

  return (
    <div className="space-y-5">
      <PageHeader
        title="Alerts"
        description="Rules are offsets from expiration, with a send time you choose rather than a fixed daily job."
        actions={
          <PrimaryButton
            disabled={!canWrite || missingChannels}
            title={
              !canWrite
                ? 'Your role cannot create alert rules'
                : missingChannels
                  ? 'Add a delivery channel first'
                  : 'Create an alert rule'
            }
            onClick={() => setEditorRule('new')}
          >
            <PlusIcon className="size-3.5" />
            New rule
          </PrimaryButton>
        }
      />

      {rules.error ? (
        <Banner tone="error" title="Could not load alert rules">
          {errorMessage(rules.error)}
        </Banner>
      ) : null}

      {missingChannels ? (
        <Banner tone="warn" title="No delivery channels configured">
          Add an email, webhook, Discord, or Telegram channel before creating a rule. Every rule must target at
          least one channel.
        </Banner>
      ) : null}

      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(320px,1fr)]">
        <Panel
          title="Offset timeline"
          description="How rules relate to the expiration date."
          action={
            <StatusPill tone="neutral" dot={false}>
              {globalCount} global
            </StatusPill>
          }
        >
          <AlertTimeline count={items.length} />
        </Panel>

        <Panel title="How delivery works" description="Designed for interrupted self-hosted instances.">
          <ul className="divide-y divide-line">
            {mechanics.map((item) => (
              <li key={item.title} className="flex gap-3 px-4 py-4 sm:px-5">
                <span className="mt-1.5 size-1.5 shrink-0 rounded-full bg-accent" aria-hidden="true" />
                <div>
                  <p className="text-xs font-semibold text-ink">{item.title}</p>
                  <p className="mt-0.5 text-[11px] leading-5 text-muted">{item.detail}</p>
                </div>
              </li>
            ))}
          </ul>
        </Panel>
      </div>

      <Panel
        title="Alert rules"
        description="Global rules apply to every domain unless a domain opts out."
        action={
          <StatusPill tone="neutral" dot={false}>
            {items.length} {items.length === 1 ? 'rule' : 'rules'}
          </StatusPill>
        }
      >
        {rules.isPending ? (
          <Spinner label="Loading alert rules" />
        ) : items.length === 0 ? (
          <div className="px-4 py-5 sm:px-5">
            <EmptyState
              icon={BellIcon}
              title="No alert rules yet"
              description="Start with a single 30-day reminder. Add a second rule for 7 days and one after expiry if renewals are slow."
              action={
                <PrimaryButton
                  disabled={!canWrite || missingChannels}
                  onClick={() => setEditorRule('new')}
                >
                  <PlusIcon className="size-3.5" />
                  Create the first rule
                </PrimaryButton>
              }
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[48rem] border-collapse text-left">
              <thead>
                <tr className="border-b border-line">
                  {['Rule', 'Scope', 'Offset', 'Send time', 'Channels', 'State', ''].map((heading, index) => (
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
                {items.map((rule) => {
                  const editable = canWrite && (rule.scope === 'domain' || isAdmin)
                  return (
                    <tr key={rule.id} className="transition-colors hover:bg-surface-2/45">
                      <td className="px-4 py-3 sm:px-5">
                        <p className="text-xs font-semibold text-ink">{rule.name}</p>
                        <p className="mt-0.5 text-[10px] text-faint">
                          Updated {new Date(rule.updated_at).toLocaleDateString()}
                        </p>
                      </td>
                      <td className="px-4 py-3 sm:px-5">
                        <StatusPill tone={rule.scope === 'global' ? 'ok' : 'neutral'} dot={false}>
                          {rule.scope}
                        </StatusPill>
                        {rule.scope === 'domain' ? (
                          <p className="mt-1 max-w-36 truncate text-[10px] text-muted">
                            {domainNames.get(rule.domain_id ?? '') ?? 'Unknown domain'}
                          </p>
                        ) : null}
                      </td>
                      <td className="px-4 py-3 text-[11px] text-ink sm:px-5">{offsetLabel(rule)}</td>
                      <td className="px-4 py-3 text-[11px] text-muted sm:px-5">{rule.send_time_local}</td>
                      <td className="px-4 py-3 sm:px-5">
                        <p className="max-w-48 text-[10px] leading-4 text-muted">
                          {rule.channel_ids.map((id) => channelNames.get(id) ?? 'Removed channel').join(', ')}
                        </p>
                      </td>
                      <td className="px-4 py-3 sm:px-5">
                        <StatusPill tone={rule.enabled ? 'ok' : 'neutral'}>
                          {rule.enabled ? 'enabled' : 'disabled'}
                        </StatusPill>
                      </td>
                      <td className="px-4 py-3 text-right sm:px-5">
                        {editable ? (
                          <div className="flex items-center justify-end gap-1">
                            <button
                              type="button"
                              onClick={() => setEditorRule(rule)}
                              className="grid size-7 place-items-center rounded-md text-muted transition-colors hover:bg-surface-2 hover:text-ink"
                              title="Edit rule"
                              aria-label={`Edit ${rule.name}`}
                            >
                              <PencilIcon className="size-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => setArchiveTarget(rule)}
                              className="grid size-7 place-items-center rounded-md text-muted transition-colors hover:bg-danger/10 hover:text-danger"
                              title="Archive rule"
                              aria-label={`Archive ${rule.name}`}
                            >
                              <TrashIcon className="size-3.5" />
                            </button>
                          </div>
                        ) : (
                          <span className="text-[10px] text-faint">Admin only</span>
                        )}
                      </td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        )}
      </Panel>

      {editorRule ? (
        <RuleDialog
          key={editorRule === 'new' ? 'new' : editorRule.id}
          rule={editorRule === 'new' ? null : editorRule}
          isAdmin={isAdmin}
          defaultTime={settings.data?.alerts.default_time ?? '09:00'}
          onClose={() => setEditorRule(null)}
          onSaved={() => {
            setEditorRule(null)
            void client.invalidateQueries({ queryKey: ['rules'] })
            void client.invalidateQueries({ queryKey: ['notifications'] })
          }}
        />
      ) : null}

      {archiveTarget ? (
        <ArchiveRuleDialog
          rule={archiveTarget}
          onClose={() => setArchiveTarget(null)}
          onArchived={() => {
            setArchiveTarget(null)
            void client.invalidateQueries({ queryKey: ['rules'] })
            void client.invalidateQueries({ queryKey: ['notifications'] })
          }}
        />
      ) : null}
    </div>
  )
}

function RuleDialog({
  rule,
  isAdmin,
  defaultTime,
  onClose,
  onSaved,
}: {
  rule: Rule | null
  isAdmin: boolean
  defaultTime: string
  onClose: () => void
  onSaved: () => void
}) {
  const domains = useDomains({ limit: 200 })
  const channels = useChannels()
  const [name, setName] = useState(rule?.name ?? '')
  const [scope, setScope] = useState<'global' | 'domain'>(rule?.scope ?? (isAdmin ? 'global' : 'domain'))
  const [domainID, setDomainID] = useState(rule?.domain_id ?? '')
  const [value, setValue] = useState(String(rule?.offset_value ?? 30))
  const [unit, setUnit] = useState<'days' | 'months'>(rule?.offset_unit ?? 'days')
  const [direction, setDirection] = useState<'before' | 'after'>(rule?.direction ?? 'before')
  const [sendTime, setSendTime] = useState(rule?.send_time_local ?? defaultTime)
  const [enabled, setEnabled] = useState(rule?.enabled ?? true)
  const [channelIDs, setChannelIDs] = useState<string[]>(rule?.channel_ids ?? [])

  const availableDomains = (domains.data?.items ?? []).filter((domain) => domain.state !== 'archived')
  const availableChannels = channels.data?.items ?? []

  const mutation = useMutation({
    mutationFn: () => {
      const payload = {
        name,
        scope,
        domain_id: scope === 'domain' ? domainID : undefined,
        offset_value: Number(value),
        offset_unit: unit,
        direction,
        send_time_local: sendTime,
        enabled,
        channel_ids: channelIDs,
      }
      if (rule) {
        return api.updateRule(rule.id, {
          name,
          offset_value: Number(value),
          offset_unit: unit,
          direction,
          send_time_local: sendTime,
          enabled,
          channel_ids: channelIDs,
        })
      }
      return api.createRule(payload)
    },
    onSuccess: onSaved,
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  const canSubmit =
    name.trim().length > 0 &&
    channelIDs.length > 0 &&
    (scope === 'global' || domainID !== '') &&
    !(Number(value) === 0 && direction === 'after') &&
    Number(value) >= 0

  return (
    <Dialog
      open
      wide
      onClose={() => {
        if (!mutation.isPending) onClose()
      }}
      title={rule ? `Edit ${rule.name}` : 'New alert rule'}
      description={
        rule
          ? 'Scope and domain cannot be changed after creation. Archive the rule and create a new one instead.'
          : 'Choose a calendar offset, a local send time, and at least one delivery channel.'
      }
      footer={
        <>
          <SecondaryButton onClick={onClose} disabled={mutation.isPending}>
            Cancel
          </SecondaryButton>
          <PrimaryButton type="submit" onClick={() => mutation.mutate()} disabled={!canSubmit || mutation.isPending}>
            {mutation.isPending ? 'Saving…' : rule ? 'Save rule' : 'Create rule'}
          </PrimaryButton>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        <FormField label="Rule name">
          <TextInput
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="30-day renewal reminder"
          />
        </FormField>

        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Scope">
            <Select
              value={scope}
              onChange={(event) => setScope(event.target.value as 'global' | 'domain')}
              disabled={Boolean(rule)}
            >
              {isAdmin ? <option value="global">Global</option> : null}
              <option value="domain">This domain</option>
            </Select>
          </FormField>
          {scope === 'domain' ? (
            <FormField label="Domain">
              <Select value={domainID} onChange={(event) => setDomainID(event.target.value)} disabled={Boolean(rule)}>
                <option value="">Choose a domain</option>
                {availableDomains.map((domain) => (
                  <option key={domain.id} value={domain.id}>
                    {domain.name}
                  </option>
                ))}
              </Select>
            </FormField>
          ) : (
            <div className="rounded-md border border-line bg-surface-2/40 px-3 py-2 text-[10px] leading-4 text-muted">
              Global rules apply to every active domain that inherits global alerts.
            </div>
          )}
        </div>

        <div>
          <p className="mb-1.5 text-[11px] font-semibold text-ink">Quick offsets</p>
          <div className="flex flex-wrap gap-1.5">
            {presets.map((preset) => (
              <button
                key={preset.label}
                type="button"
                onClick={() => {
                  setValue(String(preset.value))
                  setUnit(preset.unit)
                  setDirection(preset.direction)
                }}
                className={`rounded-full border px-2 py-1 text-[10px] font-medium transition-colors ${
                  Number(value) === preset.value && unit === preset.unit && direction === preset.direction
                    ? 'border-accent bg-accent-soft text-accent'
                    : 'border-line bg-surface text-muted hover:bg-surface-2 hover:text-ink'
                }`}
              >
                {preset.label}
              </button>
            ))}
          </div>
        </div>

        <div className="grid gap-4 sm:grid-cols-3">
          <FormField label="Send">
            <Select value={direction} onChange={(event) => setDirection(event.target.value as 'before' | 'after')}>
              <option value="before">Before expiry</option>
              <option value="after">After expiry</option>
            </Select>
          </FormField>
          <FormField label="Offset">
            <TextInput
              type="number"
              min={0}
              step={1}
              value={value}
              onChange={(event) => setValue(event.target.value)}
            />
          </FormField>
          <FormField label="Unit">
            <Select value={unit} onChange={(event) => setUnit(event.target.value as 'days' | 'months')}>
              <option value="days">Calendar days</option>
              <option value="months">Calendar months</option>
            </Select>
          </FormField>
        </div>

        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Send time" hint="Local time in the alert timezone from Settings.">
            <TextInput type="time" value={sendTime} onChange={(event) => setSendTime(event.target.value)} />
          </FormField>
          <div className="flex items-end pb-1">
            <Checkbox
              checked={enabled}
              onChange={(event) => setEnabled(event.target.checked)}
              label="Rule enabled"
              hint="Disabled rules keep their history but schedule no deliveries."
            />
          </div>
        </div>

        <div>
          <p className="mb-1.5 text-[11px] font-semibold text-ink">Delivery channels</p>
          {availableChannels.length === 0 ? (
            <Banner tone="warn">Create a channel before saving this rule.</Banner>
          ) : (
            <div className="grid gap-2 rounded-md border border-line bg-surface-2/35 p-3 sm:grid-cols-2">
              {availableChannels.map((channel) => (
                <Checkbox
                  key={channel.id}
                  checked={channelIDs.includes(channel.id)}
                  onChange={(event) => {
                    setChannelIDs((current) =>
                      event.target.checked
                        ? [...current, channel.id]
                        : current.filter((id) => id !== channel.id),
                    )
                  }}
                  label={channel.name}
                  hint={`${titleCase(channel.type)}${channel.enabled ? '' : ' · disabled'}`}
                />
              ))}
            </div>
          )}
        </div>

        {Number(value) === 0 && direction === 'after' ? (
          <Banner tone="warn">A zero-day offset is only valid before expiry. Use 1 day after for a post-expiry reminder.</Banner>
        ) : null}

        {mutation.error ? (
          <Banner tone="error" title="Rule could not be saved">
            {errorMessage(mutation.error)}
          </Banner>
        ) : null}
      </form>
    </Dialog>
  )
}

function ArchiveRuleDialog({
  rule,
  onClose,
  onArchived,
}: {
  rule: Rule
  onClose: () => void
  onArchived: () => void
}) {
  const mutation = useMutation({
    mutationFn: () => api.archiveRule(rule.id),
    onSuccess: onArchived,
  })
  return (
    <Dialog
      open
      onClose={onClose}
      title={`Archive ${rule.name}`}
      description="Pending notifications for this rule are canceled. Existing history remains visible."
      footer={
        <>
          <SecondaryButton onClick={onClose}>Cancel</SecondaryButton>
          <DangerButton onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            <TrashIcon className="size-3.5" />
            {mutation.isPending ? 'Archiving…' : 'Archive rule'}
          </DangerButton>
        </>
      }
    >
      {mutation.error ? (
        <Banner tone="error" title="Archive failed">
          {errorMessage(mutation.error)}
        </Banner>
      ) : (
        <p className="text-xs leading-5 text-muted">
          The rule will stop generating new alerts for {rule.scope === 'global' ? 'every inherited domain' : 'its domain'}.
        </p>
      )}
    </Dialog>
  )
}
