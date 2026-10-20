import { useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  api,
  useAudit,
  useJobs,
  useMe,
  useMeta,
  useSettings,
  useUsers,
  type Role,
  type Settings,
  type User,
} from '../lib/api'
import {
  formatDateTime,
  formatVersion,
  fromDateTimeLocal,
  parseRecipients,
  relativeTime,
  shortCommit,
  titleCase,
  toDateTimeLocal,
} from '../lib/format'
import {
  CheckIcon,
  DatabaseIcon,
  ShieldIcon,
} from '../components/icons'
import {
  Banner,
  DefinitionRow,
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

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return 'The request could not be completed.'
}

function Code({ children }: { children: string }) {
  return (
    <code className="rounded border border-line bg-surface-2 px-1 py-0.5 font-mono text-[10px] text-ink">
      {children}
    </code>
  )
}

type Tab = 'runtime' | 'users' | 'audit' | 'jobs'

const tabs: Array<{ id: Tab; label: string }> = [
  { id: 'runtime', label: 'Runtime settings' },
  { id: 'users', label: 'Users' },
  { id: 'audit', label: 'Audit log' },
  { id: 'jobs', label: 'Jobs' },
]

export function SettingsPage() {
  const meta = useMeta()
  const me = useMe()
  const settings = useSettings()
  const isAdmin = me.data?.user?.role === 'admin'
  const [tab, setTab] = useState<Tab>('runtime')

  const activeTab: Tab = isAdmin ? tab : 'runtime'
  const authConfigured = meta.data?.auth_configured ?? false
  const tlsEnabled = meta.data?.tls_enabled ?? false

  return (
    <div className="space-y-5">
      <PageHeader
        title="Settings"
        description="Runtime delivery, scheduling, and access controls for this instance."
        actions={
          <StatusPill tone={isAdmin ? 'ok' : 'neutral'} dot={false}>
            {isAdmin ? 'Administrator' : 'Read only'}
          </StatusPill>
        }
      />

      {settings.error ? (
        <Banner tone="error" title="Runtime settings could not be loaded">
          {errorMessage(settings.error)}
        </Banner>
      ) : null}

      <div className="grid gap-5 lg:grid-cols-2">
        <Panel title="Instance" description="What this process is doing right now." bodyClassName="px-4 sm:px-5">
          <dl>
            <DefinitionRow label="Authentication">
              <div className="flex flex-wrap items-center gap-2">
                <StatusPill tone={authConfigured ? 'ok' : 'warn'}>
                  {authConfigured ? 'OIDC configured' : 'OIDC not configured'}
                </StatusPill>
                <span className="text-[11px] text-muted">
                  {meta.data?.provider ? titleCase(meta.data.provider) : 'No provider preset'}
                  {meta.data?.issuer ? ` · ${meta.data.issuer}` : ''}
                </span>
              </div>
            </DefinitionRow>
            <DefinitionRow label="Built-in TLS">
              <StatusPill tone={tlsEnabled ? 'ok' : 'neutral'}>
                {tlsEnabled ? 'Enabled' : 'Disabled, reverse proxy expected'}
              </StatusPill>
            </DefinitionRow>
            <DefinitionRow label="Storage">
              <div className="flex items-center gap-2">
                <DatabaseIcon className="size-3.5 text-faint" />
                <span>SQLite with WAL. One writer, pooled readers.</span>
              </div>
            </DefinitionRow>
            <DefinitionRow label="Instance lock">
              <div className="flex items-center gap-2">
                <ShieldIcon className="size-3.5 text-faint" />
                <span>One IWMMD process per data directory.</span>
              </div>
            </DefinitionRow>
            <DefinitionRow label="Version">
              <span className="tnum">{formatVersion(meta.data?.version)}</span>
            </DefinitionRow>
            <DefinitionRow label="Commit">
              <span className="font-mono">{shortCommit(meta.data?.commit)}</span>
            </DefinitionRow>
            <DefinitionRow label="Built">
              <span className="tnum">{meta.data?.date ?? 'unknown'}</span>
            </DefinitionRow>
          </dl>
        </Panel>

        <Panel title="Configuration" description="How this instance is shaped at startup." bodyClassName="px-4 sm:px-5">
          <dl>
            <DefinitionRow label="Config file">
              <Code>/etc/iwmmd/config.toml</Code> when present, otherwise defaults.
            </DefinitionRow>
            <DefinitionRow label="Environment">
              <Code>IWMMD_</Code> prefixed variables override the file.
            </DefinitionRow>
            <DefinitionRow label="Precedence">
              environment, then config file, then built-in defaults.
            </DefinitionRow>
            <DefinitionRow label="Master key">
              <Code>IWMMD_SECRET_KEY</Code> or <Code>IWMMD_SECRET_KEY_FILE</Code>. Generate with{' '}
              <Code>iwmmd gen-key</Code>.
            </DefinitionRow>
            <DefinitionRow label="Authorization">
              OIDC claims first, then a verified email domain, then an explicit email. Admin access requires an
              admin group or named admin email.
            </DefinitionRow>
            <DefinitionRow label="Delivery">
              SMTP, generic webhook, Discord, and Telegram. Secrets are AES-256-GCM encrypted at rest.
            </DefinitionRow>
          </dl>
        </Panel>
      </div>

      {isAdmin ? (
        <div className="flex gap-1 overflow-x-auto rounded-md border border-line bg-surface p-1">
          {tabs.map((item) => (
            <button
              key={item.id}
              type="button"
              onClick={() => setTab(item.id)}
              className={`shrink-0 rounded px-3 py-1.5 text-[11px] font-semibold transition-colors ${
                activeTab === item.id ? 'bg-accent-soft text-accent' : 'text-muted hover:bg-surface-2 hover:text-ink'
              }`}
            >
              {item.label}
            </button>
          ))}
        </div>
      ) : null}

      {activeTab === 'runtime' ? (
        settings.isPending ? (
          <Panel>
            <Spinner label="Loading runtime settings" />
          </Panel>
        ) : settings.data ? (
          isAdmin ? (
            <RuntimeSettings settings={settings.data} />
          ) : (
            <ReadonlySettings settings={settings.data} />
          )
        ) : null
      ) : null}

      {activeTab === 'users' ? <UsersPanel /> : null}
      {activeTab === 'audit' ? <AuditPanel /> : null}
      {activeTab === 'jobs' ? <JobsPanel /> : null}
    </div>
  )
}

function RuntimeSettings({ settings }: { settings: Settings }) {
  return (
    <div className="space-y-5">
      <SMTPForm settings={settings} />
      <div className="grid items-start gap-5 lg:grid-cols-2">
        <AlertsForm settings={settings} />
        <CheckForm settings={settings} />
      </div>
      <RetentionForm settings={settings} />
    </div>
  )
}

function ReadonlySettings({ settings }: { settings: Settings }) {
  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <Panel title="SMTP" description="Outbound email for email channels and test messages." bodyClassName="px-4 sm:px-5">
        <dl>
          <DefinitionRow label="Host">{settings.smtp.host ? `${settings.smtp.host}:${settings.smtp.port}` : 'Not configured'}</DefinitionRow>
          <DefinitionRow label="TLS mode">{settings.smtp.tls_mode}</DefinitionRow>
          <DefinitionRow label="From">{settings.smtp.from || 'Not configured'}</DefinitionRow>
          <DefinitionRow label="Password">{settings.smtp.password_set ? 'Stored' : 'Not set'}</DefinitionRow>
        </dl>
      </Panel>
      <Panel title="Alert defaults" description="Scheduling defaults applied to new rules and catch-up." bodyClassName="px-4 sm:px-5">
        <dl>
          <DefinitionRow label="Timezone">{settings.alerts.timezone}</DefinitionRow>
          <DefinitionRow label="Default send time">{settings.alerts.default_time}</DefinitionRow>
          <DefinitionRow label="Global pause">
            {settings.alerts.paused_until ? formatDateTime(settings.alerts.paused_until) : 'Not paused'}
          </DefinitionRow>
        </dl>
      </Panel>
      <Panel title="Checks" description="Scheduling and manual check rate limit." bodyClassName="px-4 sm:px-5">
        <dl>
          <DefinitionRow label="Default interval">{formatDuration(settings.check.default_interval)}</DefinitionRow>
          <DefinitionRow label="Manual rate limit">{formatDuration(settings.check.manual_rate_limit)}</DefinitionRow>
        </dl>
      </Panel>
      <Panel title="Retention" description="How long check and delivery history is kept." bodyClassName="px-4 sm:px-5">
        <dl>
          <DefinitionRow label="Raw responses">{settings.retention.raw_days} days</DefinitionRow>
          <DefinitionRow label="Check history">{settings.retention.checks_days} days</DefinitionRow>
          <DefinitionRow label="Notifications">{settings.retention.notifications_days} days</DefinitionRow>
        </dl>
      </Panel>
    </div>
  )
}

function formatDuration(seconds: number): string {
  if (seconds % 86400 === 0) return `${seconds / 86400} days`
  if (seconds % 3600 === 0) return `${seconds / 3600} hours`
  if (seconds % 60 === 0) return `${seconds / 60} minutes`
  return `${seconds} seconds`
}

function SMTPForm({ settings }: { settings: Settings }) {
  const client = useQueryClient()
  const [host, setHost] = useState(settings.smtp.host)
  const [port, setPort] = useState(String(settings.smtp.port || 587))
  const [tlsMode, setTLSMode] = useState(settings.smtp.tls_mode || 'starttls')
  const [username, setUsername] = useState(settings.smtp.username)
  const [password, setPassword] = useState('')
  const [from, setFrom] = useState(settings.smtp.from)
  const [testTo, setTestTo] = useState('')
  const [notice, setNotice] = useState<{ tone: 'ok' | 'error'; message: string } | null>(null)

  const save = useMutation({
    mutationFn: () =>
      api.saveSMTP({
        host,
        port: Number(port),
        tls_mode: tlsMode,
        username,
        ...(password ? { password } : {}),
        from,
      }),
    onSuccess: () => {
      setPassword('')
      setNotice({ tone: 'ok', message: 'SMTP settings saved.' })
      void client.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const test = useMutation({
    mutationFn: () => api.testSMTP(parseRecipients(testTo)),
    onSuccess: () => setNotice({ tone: 'ok', message: 'Test email sent.' }),
    onError: (error) => setNotice({ tone: 'error', message: errorMessage(error) }),
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    save.mutate()
  }

  return (
    <Panel
      title="SMTP"
      description="Outbound mail for email channels and test messages."
      action={
        settings.smtp.managed_by_env ? (
          <StatusPill tone="warn" dot={false}>Managed by environment</StatusPill>
        ) : (
          <StatusPill tone={settings.smtp.password_set ? 'ok' : 'neutral'} dot={false}>
            {settings.smtp.password_set ? 'Password set' : 'No password'}
          </StatusPill>
        )
      }
    >
      <form onSubmit={submit} className="space-y-4 px-4 py-4 sm:px-5">
        {settings.smtp.managed_by_env ? (
          <Banner tone="warn" title="Environment overrides">
            SMTP values are controlled by IWMMD environment variables. Remove the overrides to edit them here.
          </Banner>
        ) : null}
        {notice ? (
          <Banner tone={notice.tone} title={notice.tone === 'ok' ? 'Saved' : 'SMTP error'}>
            {notice.message}
          </Banner>
        ) : null}
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_7rem]">
          <FormField label="Host">
            <TextInput value={host} onChange={(event) => setHost(event.target.value)} placeholder="smtp.example.com" disabled={settings.smtp.managed_by_env} />
          </FormField>
          <FormField label="Port">
            <TextInput type="number" min={1} max={65535} value={port} onChange={(event) => setPort(event.target.value)} disabled={settings.smtp.managed_by_env} />
          </FormField>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="TLS mode">
            <Select value={tlsMode} onChange={(event) => setTLSMode(event.target.value)} disabled={settings.smtp.managed_by_env}>
              <option value="starttls">STARTTLS</option>
              <option value="implicit">Implicit TLS</option>
              <option value="none">No TLS</option>
            </Select>
          </FormField>
          <FormField label="From address">
            <TextInput type="email" value={from} onChange={(event) => setFrom(event.target.value)} placeholder="iwmmd@example.com" disabled={settings.smtp.managed_by_env} />
          </FormField>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Username">
            <TextInput value={username} onChange={(event) => setUsername(event.target.value)} autoComplete="username" disabled={settings.smtp.managed_by_env} />
          </FormField>
          <FormField label="Password" hint={settings.smtp.password_set ? 'Leave blank to keep the stored password.' : 'No password stored.'}>
            <TextInput
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="new-password"
              placeholder={settings.smtp.password_set ? 'Leave blank to keep current' : 'Optional'}
              disabled={settings.smtp.managed_by_env}
            />
          </FormField>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <PrimaryButton type="submit" disabled={save.isPending || settings.smtp.managed_by_env}>
            <CheckIcon className="size-3.5" />
            {save.isPending ? 'Saving…' : 'Save SMTP'}
          </PrimaryButton>
          <TextInput
            value={testTo}
            onChange={(event) => setTestTo(event.target.value)}
            placeholder="Test recipient, defaults to From"
            aria-label="Test recipient"
            className="max-w-xs"
          />
          <SecondaryButton
            onClick={() => test.mutate()}
            disabled={test.isPending || (!host && !settings.smtp.host)}
          >
            {test.isPending ? 'Sending…' : 'Send test'}
          </SecondaryButton>
        </div>
        {save.error ? <Banner tone="error" title="SMTP could not be saved">{errorMessage(save.error)}</Banner> : null}
      </form>
    </Panel>
  )
}

function AlertsForm({ settings }: { settings: Settings }) {
  const client = useQueryClient()
  const [timezone, setTimezone] = useState(settings.alerts.timezone)
  const [defaultTime, setDefaultTime] = useState(settings.alerts.default_time)
  const [pausedUntil, setPausedUntil] = useState(toDateTimeLocal(settings.alerts.paused_until))
  const [saved, setSaved] = useState(false)

  const mutation = useMutation({
    mutationFn: () =>
      api.saveAlerts({
        timezone,
        default_time: defaultTime,
        paused_until: pausedUntil ? fromDateTimeLocal(pausedUntil) : null,
      }),
    onSuccess: () => {
      setSaved(true)
      void client.invalidateQueries({ queryKey: ['settings'] })
      void client.invalidateQueries({ queryKey: ['notifications'] })
    },
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <Panel title="Alert defaults" description="Timezone, default send time, and the global pause gate.">
      <form onSubmit={submit} className="space-y-4 px-4 py-4 sm:px-5">
        {settings.alerts.managed_by_env ? (
          <Banner tone="warn">Alert defaults are controlled by the environment.</Banner>
        ) : null}
        {saved ? <Banner tone="ok" title="Saved">Pending alerts have been reconciled with the new schedule.</Banner> : null}
        <FormField label="Timezone" hint="IANA name, for example Europe/Berlin or America/New_York.">
          <TextInput
            value={timezone}
            onChange={(event) => setTimezone(event.target.value)}
            list="iwmmd-timezones"
            disabled={settings.alerts.managed_by_env}
          />
          <datalist id="iwmmd-timezones">
            {['UTC', 'Europe/London', 'Europe/Berlin', 'Europe/Paris', 'America/New_York', 'America/Chicago', 'America/Los_Angeles', 'Asia/Tokyo', 'Australia/Sydney'].map((zone) => (
              <option key={zone} value={zone} />
            ))}
          </datalist>
        </FormField>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Default send time" hint="Used when a rule leaves its send time blank.">
            <TextInput type="time" value={defaultTime} onChange={(event) => setDefaultTime(event.target.value)} disabled={settings.alerts.managed_by_env} />
          </FormField>
          <FormField label="Global pause until" hint="Leave blank to resume all alerts.">
            <TextInput type="datetime-local" value={pausedUntil} onChange={(event) => setPausedUntil(event.target.value)} disabled={settings.alerts.managed_by_env} />
          </FormField>
        </div>
        <PrimaryButton type="submit" disabled={mutation.isPending || settings.alerts.managed_by_env}>
          {mutation.isPending ? 'Saving…' : 'Save alert defaults'}
        </PrimaryButton>
        {mutation.error ? <Banner tone="error" title="Settings could not be saved">{errorMessage(mutation.error)}</Banner> : null}
      </form>
    </Panel>
  )
}

function CheckForm({ settings }: { settings: Settings }) {
  const client = useQueryClient()
  const [interval, setInterval] = useState(String(settings.check.default_interval))
  const [manual, setManual] = useState(String(settings.check.manual_rate_limit))
  const [saved, setSaved] = useState(false)

  const mutation = useMutation({
    mutationFn: () =>
      api.saveCheck({
        default_interval: Number(interval),
        manual_rate_limit: Number(manual),
      }),
    onSuccess: () => {
      setSaved(true)
      void client.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <Panel title="Checks" description="Scheduler cadence and the manual check cooldown.">
      <form onSubmit={submit} className="space-y-4 px-4 py-4 sm:px-5">
        {settings.check.managed_by_env ? <Banner tone="warn">Check settings are controlled by the environment.</Banner> : null}
        {saved ? <Banner tone="ok" title="Saved">New domains use the updated default interval.</Banner> : null}
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Default interval" hint="Seconds between scheduled checks. Minimum 300.">
            <TextInput type="number" min={300} step={300} value={interval} onChange={(event) => setInterval(event.target.value)} disabled={settings.check.managed_by_env} />
          </FormField>
          <FormField label="Manual rate limit" hint="Seconds before a domain can be checked manually again. Minimum 60.">
            <TextInput type="number" min={60} step={60} value={manual} onChange={(event) => setManual(event.target.value)} disabled={settings.check.managed_by_env} />
          </FormField>
        </div>
        <PrimaryButton type="submit" disabled={mutation.isPending || settings.check.managed_by_env}>
          {mutation.isPending ? 'Saving…' : 'Save check settings'}
        </PrimaryButton>
        {mutation.error ? <Banner tone="error" title="Settings could not be saved">{errorMessage(mutation.error)}</Banner> : null}
      </form>
    </Panel>
  )
}

function RetentionForm({ settings }: { settings: Settings }) {
  const client = useQueryClient()
  const [raw, setRaw] = useState(String(settings.retention.raw_days))
  const [checks, setChecks] = useState(String(settings.retention.checks_days))
  const [notifications, setNotifications] = useState(String(settings.retention.notifications_days))
  const [saved, setSaved] = useState(false)

  const mutation = useMutation({
    mutationFn: () =>
      api.saveRetention({
        raw_days: Number(raw),
        checks_days: Number(checks),
        notifications_days: Number(notifications),
      }),
    onSuccess: () => {
      setSaved(true)
      void client.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <Panel title="Retention" description="Automatic cleanup bounds for check history and delivery records.">
      <form onSubmit={submit} className="space-y-4 px-4 py-4 sm:px-5">
        {settings.retention.managed_by_env ? <Banner tone="warn">Retention is controlled by the environment.</Banner> : null}
        {saved ? <Banner tone="ok" title="Saved">Retention changes apply to the next cleanup pass.</Banner> : null}
        <div className="grid gap-4 sm:grid-cols-3">
          <FormField label="Raw responses" hint="Days to keep RDAP and WHOIS bodies.">
            <TextInput type="number" min={1} value={raw} onChange={(event) => setRaw(event.target.value)} disabled={settings.retention.managed_by_env} />
          </FormField>
          <FormField label="Check history" hint="Days to keep check records.">
            <TextInput type="number" min={1} value={checks} onChange={(event) => setChecks(event.target.value)} disabled={settings.retention.managed_by_env} />
          </FormField>
          <FormField label="Notifications" hint="Days to keep delivery history.">
            <TextInput type="number" min={1} value={notifications} onChange={(event) => setNotifications(event.target.value)} disabled={settings.retention.managed_by_env} />
          </FormField>
        </div>
        <PrimaryButton type="submit" disabled={mutation.isPending || settings.retention.managed_by_env}>
          {mutation.isPending ? 'Saving…' : 'Save retention'}
        </PrimaryButton>
        {mutation.error ? <Banner tone="error" title="Settings could not be saved">{errorMessage(mutation.error)}</Banner> : null}
      </form>
    </Panel>
  )
}

function UsersPanel() {
  const client = useQueryClient()
  const me = useMe()
  const users = useUsers()
  const [notice, setNotice] = useState<{ tone: 'ok' | 'error'; message: string } | null>(null)

  const setRole = useMutation({
    mutationFn: ({ user, role }: { user: User; role: Role }) => api.setUserRole(user.id, role),
    onSuccess: (_data, variables) => {
      setNotice({ tone: 'ok', message: `Role updated for ${variables.user.display_name || variables.user.email}.` })
      void client.invalidateQueries({ queryKey: ['users'] })
      void client.invalidateQueries({ queryKey: ['me'] })
    },
    onError: (error) => setNotice({ tone: 'error', message: errorMessage(error) }),
  })

  const revoke = useMutation({
    mutationFn: (user: User) => api.revokeUserSessions(user.id),
    onSuccess: () => setNotice({ tone: 'ok', message: 'User sessions revoked.' }),
    onError: (error) => setNotice({ tone: 'error', message: errorMessage(error) }),
  })

  return (
    <Panel title="Users" description="Accounts are provisioned on first OIDC sign-in. Roles are stored per user.">
      {notice ? (
        <div className="px-4 pt-4 sm:px-5">
          <Banner tone={notice.tone} title={notice.tone === 'ok' ? 'Done' : 'User action failed'}>
            {notice.message}
          </Banner>
        </div>
      ) : null}
      {users.isPending ? (
        <Spinner label="Loading users" />
      ) : (users.data?.items.length ?? 0) === 0 ? (
        <div className="px-4 py-5 text-[11px] text-muted sm:px-5">No users have signed in yet.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[42rem] border-collapse text-left">
            <thead>
              <tr className="border-b border-line">
                {['User', 'Role', 'Last sign-in', 'Created', ''].map((heading, index) => (
                  <th key={`${heading}-${index}`} className="px-4 py-2.5 text-[10px] font-semibold text-faint last:text-right sm:px-5">
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {users.data?.items.map((user) => (
                <tr key={user.id} className="transition-colors hover:bg-surface-2/45">
                  <td className="px-4 py-3 sm:px-5">
                    <p className="text-xs font-semibold text-ink">{user.display_name || user.email}</p>
                    <p className="mt-0.5 text-[10px] text-faint">{user.email}</p>
                  </td>
                  <td className="px-4 py-3 sm:px-5">
                    <Select
                      value={user.role}
                      onChange={(event) => setRole.mutate({ user, role: event.target.value as Role })}
                      disabled={setRole.isPending}
                      className="w-28"
                      aria-label={`Role for ${user.email}`}
                    >
                      <option value="viewer">Viewer</option>
                      <option value="member">Member</option>
                      <option value="admin">Admin</option>
                    </Select>
                  </td>
                  <td className="px-4 py-3 text-[10px] text-muted sm:px-5">{relativeTime(user.last_login_at)}</td>
                  <td className="px-4 py-3 text-[10px] text-muted sm:px-5">{formatDateTime(user.created_at)}</td>
                  <td className="px-4 py-3 text-right sm:px-5">
                    <SecondaryButton
                      onClick={() => {
                        if (window.confirm(`Revoke all sessions for ${user.email}?`)) revoke.mutate(user)
                      }}
                      disabled={revoke.isPending}
                    >
                      Revoke sessions
                    </SecondaryButton>
                    {me.data?.user?.id === user.id ? (
                      <p className="mt-1 text-[9px] text-faint">current session kept</p>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Panel>
  )
}

function AuditPanel() {
  const audit = useAudit(150)
  return (
    <Panel title="Audit log" description="Immutable records for sign-ins, writes, mutes, retries, and role changes.">
      {audit.isPending ? (
        <Spinner label="Loading audit entries" />
      ) : (audit.data?.items.length ?? 0) === 0 ? (
        <div className="px-4 py-5 text-[11px] text-muted sm:px-5">No audit entries yet.</div>
      ) : (
        <ul className="divide-y divide-line">
          {audit.data?.items.map((entry) => (
            <li key={entry.id} className="px-4 py-3 sm:px-5">
              <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                  <p className="text-[11px] font-semibold text-ink">{entry.action}</p>
                  <p className="mt-0.5 text-[10px] text-faint">
                    {entry.entity_type}
                    {entry.entity_id ? ` · ${entry.entity_id}` : ''}
                    {entry.actor_user_id ? ` · actor ${entry.actor_user_id.slice(0, 8)}` : ''}
                    {entry.ip ? ` · ${entry.ip}` : ''}
                  </p>
                </div>
                <p className="text-[10px] text-faint">{formatDateTime(entry.created_at)}</p>
              </div>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  )
}

function JobsPanel() {
  const jobs = useJobs(100)
  return (
    <Panel title="Jobs" description="Durable scheduler state for domain checks and future background work.">
      {jobs.isPending ? (
        <Spinner label="Loading jobs" />
      ) : (jobs.data?.items.length ?? 0) === 0 ? (
        <div className="px-4 py-5 text-[11px] text-muted sm:px-5">No jobs recorded yet.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[46rem] border-collapse text-left">
            <thead>
              <tr className="border-b border-line">
                {['Job', 'State', 'Run at', 'Attempts', 'Error'].map((heading) => (
                  <th key={heading} className="px-4 py-2.5 text-[10px] font-semibold text-faint sm:px-5">
                    {heading}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {jobs.data?.items.map((job) => (
                <tr key={job.id} className="transition-colors hover:bg-surface-2/45">
                  <td className="px-4 py-3 sm:px-5">
                    <p className="text-[11px] font-semibold text-ink">{job.kind}</p>
                    <p className="mt-0.5 font-mono text-[9px] text-faint">{job.unique_key || job.id}</p>
                  </td>
                  <td className="px-4 py-3 sm:px-5">
                    <StatusPill
                      tone={
                        job.state === 'succeeded'
                          ? 'ok'
                          : job.state === 'dead' || job.state === 'failed'
                            ? 'error'
                            : job.state === 'running' || job.state === 'pending'
                              ? 'warn'
                              : 'neutral'
                      }
                      dot={false}
                    >
                      {job.state}
                    </StatusPill>
                  </td>
                  <td className="px-4 py-3 text-[10px] text-muted sm:px-5">{formatDateTime(job.run_at)}</td>
                  <td className="px-4 py-3 text-[10px] text-muted sm:px-5">
                    {job.attempts} / {job.max_attempts}
                  </td>
                  <td className="px-4 py-3 text-[10px] text-danger sm:px-5">{job.last_error || '-'}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Panel>
  )
}
