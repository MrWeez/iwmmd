import { useMemo, useState, type ComponentType, type FormEvent, type SVGProps } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  ApiError,
  api,
  useChannels,
  useMe,
  type Channel,
  type ChannelType,
} from '../lib/api'
import {
  formatDateTime,
  fromDateTimeLocal,
  parseRecipients,
  titleCase,
  toDateTimeLocal,
} from '../lib/format'
import {
  BellIcon,
  ChatIcon,
  ClockIcon,
  MailIcon,
  PauseIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  SendIcon,
  TrashIcon,
  WebhookIcon,
} from '../components/icons'
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
  Spinner,
  StatusPill,
  Textarea,
  TextInput,
} from '../components/ui'

type IconComponent = ComponentType<SVGProps<SVGSVGElement>>

const descriptions: Record<ChannelType, string> = {
  email: 'SMTP delivery to one or more recipients.',
  webhook: 'A JSON POST to any HTTPS endpoint.',
  discord: 'A message through an incoming Discord webhook URL.',
  telegram: 'A bot message to a chat, group, or forum topic.',
}

const icons: Record<ChannelType, IconComponent> = {
  email: MailIcon,
  webhook: WebhookIcon,
  discord: ChatIcon,
  telegram: SendIcon,
}

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return 'The request could not be completed.'
}

function summaryLine(channel: Channel): string {
  const config = channel.config
  switch (channel.type) {
    case 'email':
      return Array.isArray(config.recipients) ? (config.recipients as string[]).join(', ') : 'No recipients'
    case 'webhook':
      return `${String(config.host ?? 'Endpoint')}${config.allow_private ? ' · private network allowed' : ''}`
    case 'discord':
      return String(config.host ?? 'Discord webhook')
    case 'telegram':
      return `Chat ${String(config.chat_id ?? 'unknown')}${config.thread_id ? ` · topic ${String(config.thread_id)}` : ''}`
  }
}

export function ChannelsPage() {
  const client = useQueryClient()
  const me = useMe()
  const isAdmin = me.data?.user?.role === 'admin'
  const channels = useChannels()
  const [editor, setEditor] = useState<Channel | 'new' | null>(null)
  const [muteTarget, setMuteTarget] = useState<Channel | null>(null)
  const [archiveTarget, setArchiveTarget] = useState<Channel | null>(null)
  const [testingID, setTestingID] = useState<string | null>(null)
  const [testResult, setTestResult] = useState<{ tone: 'ok' | 'error'; message: string } | null>(null)

  const testMutation = useMutation({
    mutationFn: (id: string) => api.testChannel(id),
    onMutate: (id) => {
      setTestingID(id)
      setTestResult(null)
    },
    onSuccess: (_data, id) => {
      const channel = channels.data?.items.find((item) => item.id === id)
      setTestResult({ tone: 'ok', message: `Test delivered through ${channel?.name ?? 'the channel'}.` })
    },
    onError: (error) => {
      const message = error instanceof ApiError && error.detail ? error.detail : errorMessage(error)
      setTestResult({ tone: 'error', message })
    },
    onSettled: () => setTestingID(null),
  })

  const refresh = () => {
    void client.invalidateQueries({ queryKey: ['channels'] })
    void client.invalidateQueries({ queryKey: ['notifications'] })
  }

  const items = channels.data?.items ?? []
  const enabled = items.filter((channel) => channel.enabled).length
  const system = items.filter((channel) => channel.receive_system_alerts).length

  return (
    <div className="space-y-5">
      <PageHeader
        title="Channels"
        description="Where expiry and system alerts are delivered. Attach one or more channels to any rule."
        actions={
          <PrimaryButton
            disabled={!isAdmin}
            title={isAdmin ? 'Add a delivery channel' : 'Only admins can manage channels'}
            onClick={() => setEditor('new')}
          >
            <PlusIcon className="size-3.5" />
            Add channel
          </PrimaryButton>
        }
      />

      {!isAdmin ? (
        <Banner tone="neutral" title="Read-only channel view">
          Your role can inspect delivery routes but only an administrator can change them.
        </Banner>
      ) : null}

      {channels.error ? (
        <Banner tone="error" title="Could not load channels">
          {errorMessage(channels.error)}
        </Banner>
      ) : null}

      {testResult ? (
        <Banner tone={testResult.tone} title={testResult.tone === 'ok' ? 'Test sent' : 'Test failed'}>
          {testResult.message}
        </Banner>
      ) : null}

      <Panel
        title="Delivery channels"
        description="Secrets are encrypted at rest with the instance master key."
        action={
          <div className="flex items-center gap-2">
            <StatusPill tone="neutral" dot={false}>
              {enabled} enabled
            </StatusPill>
            <StatusPill tone="neutral" dot={false}>
              {system} system
            </StatusPill>
          </div>
        }
      >
        {channels.isPending ? (
          <Spinner label="Loading channels" />
        ) : items.length === 0 ? (
          <div className="px-4 py-5 sm:px-5">
            <EmptyState
              icon={BellIcon}
              title="No delivery channels yet"
              description="Add email, a webhook, Discord, or Telegram. Every alert rule needs at least one channel."
              action={
                <PrimaryButton disabled={!isAdmin} onClick={() => setEditor('new')}>
                  <PlusIcon className="size-3.5" />
                  Add channel
                </PrimaryButton>
              }
            />
          </div>
        ) : (
          <ul className="divide-y divide-line">
            {items.map((channel) => {
              const Icon = icons[channel.type]
              return (
                <li key={channel.id} className="px-4 py-4 sm:px-5">
                  <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                    <div className="flex min-w-0 gap-3">
                      <span className="grid size-9 shrink-0 place-items-center rounded-md border border-line bg-surface-2 text-muted">
                        <Icon className="size-4" />
                      </span>
                      <div className="min-w-0">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-xs font-semibold text-ink">{channel.name}</p>
                          <StatusPill tone={channel.enabled ? 'ok' : 'neutral'}>
                            {channel.enabled ? 'enabled' : 'disabled'}
                          </StatusPill>
                          {channel.receive_system_alerts ? (
                            <StatusPill tone="warn" dot={false}>
                              system alerts
                            </StatusPill>
                          ) : null}
                          {channel.muted_until ? (
                            <StatusPill tone="warn" dot={false}>
                              muted
                            </StatusPill>
                          ) : null}
                        </div>
                        <p className="mt-0.5 text-[11px] leading-5 text-muted">{descriptions[channel.type]}</p>
                        <p className="mt-0.5 break-words text-[10px] leading-5 text-faint">{summaryLine(channel)}</p>
                        {channel.muted_until ? (
                          <p className="mt-1 text-[10px] text-signal">
                            Muted until {formatDateTime(channel.muted_until)}
                            {channel.mute_reason ? ` · ${channel.mute_reason}` : ''}
                          </p>
                        ) : null}
                      </div>
                    </div>

                    {isAdmin ? (
                      <div className="flex shrink-0 flex-wrap items-center gap-1.5 sm:justify-end">
                        <SecondaryButton
                          onClick={() => testMutation.mutate(channel.id)}
                          disabled={!channel.enabled || testingID === channel.id}
                          title="Send a test notification"
                        >
                          <SendIcon className="size-3.5" />
                          {testingID === channel.id ? 'Sending…' : 'Test'}
                        </SecondaryButton>
                        <button
                          type="button"
                          onClick={() => setMuteTarget(channel)}
                          className="grid size-8 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-ink"
                          title={channel.muted_until ? 'Change mute' : 'Mute channel'}
                          aria-label={`Mute ${channel.name}`}
                        >
                          <PauseIcon className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setEditor(channel)}
                          className="grid size-8 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-ink"
                          title="Edit channel"
                          aria-label={`Edit ${channel.name}`}
                        >
                          <PencilIcon className="size-3.5" />
                        </button>
                        <button
                          type="button"
                          onClick={() => setArchiveTarget(channel)}
                          className="grid size-8 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-danger/10 hover:text-danger"
                          title="Archive channel"
                          aria-label={`Archive ${channel.name}`}
                        >
                          <TrashIcon className="size-3.5" />
                        </button>
                      </div>
                    ) : null}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
        <div className="border-t border-line bg-surface-2/50 px-4 py-3 text-[10px] leading-5 text-faint sm:px-5">
          Each channel can be muted independently. A muted channel still records queued notifications and catches
          up with the latest due alert when it is unmuted.
        </div>
      </Panel>

      <div className="grid gap-5 sm:grid-cols-2">
        <Panel>
          <div className="flex gap-3 px-4 py-4 sm:px-5">
            <span className="grid size-9 shrink-0 place-items-center rounded-md border border-line bg-surface-2 text-muted">
              <ClockIcon className="size-4" />
            </span>
            <div>
              <h2 className="text-xs font-semibold text-ink">Failed sends become incidents</h2>
              <p className="mt-1 text-[11px] leading-5 text-muted">
                Retries use exponential backoff. After the final attempt, the failure appears in Incidents for
                inspection and manual retry.
              </p>
            </div>
          </div>
        </Panel>
        <Panel>
          <div className="flex gap-3 px-4 py-4 sm:px-5">
            <span className="grid size-9 shrink-0 place-items-center rounded-md border border-line bg-surface-2 text-muted">
              <BellIcon className="size-4" />
            </span>
            <div>
              <h2 className="text-xs font-semibold text-ink">Muting is scoped</h2>
              <p className="mt-1 text-[11px] leading-5 text-muted">
                Pause a domain, a channel, or everything. Delivery holds until the mute ends, then the latest due
                alert sends instead of the whole backlog.
              </p>
            </div>
          </div>
        </Panel>
      </div>

      {editor ? (
        <ChannelDialog
          key={editor === 'new' ? 'new' : editor.id}
          channel={editor === 'new' ? null : editor}
          onClose={() => setEditor(null)}
          onSaved={() => {
            setEditor(null)
            refresh()
          }}
        />
      ) : null}

      {muteTarget ? (
        <MuteChannelDialog
          key={muteTarget.id}
          channel={muteTarget}
          onClose={() => setMuteTarget(null)}
          onSaved={() => {
            setMuteTarget(null)
            refresh()
          }}
        />
      ) : null}

      {archiveTarget ? (
        <ArchiveChannelDialog
          key={archiveTarget.id}
          channel={archiveTarget}
          onClose={() => setArchiveTarget(null)}
          onArchived={() => {
            setArchiveTarget(null)
            refresh()
          }}
        />
      ) : null}
    </div>
  )
}

function ChannelDialog({
  channel,
  onClose,
  onSaved,
}: {
  channel: Channel | null
  onClose: () => void
  onSaved: () => void
}) {
  const [name, setName] = useState(channel?.name ?? '')
  const [type, setType] = useState<ChannelType>(channel?.type ?? 'email')
  const [enabled, setEnabled] = useState(channel?.enabled ?? true)
  const [systemAlerts, setSystemAlerts] = useState(channel?.receive_system_alerts ?? true)
  const [replaceConfig, setReplaceConfig] = useState(!channel)

  const [recipients, setRecipients] = useState(
    channel?.type === 'email' && Array.isArray(channel.config.recipients)
      ? (channel.config.recipients as string[]).join('\n')
      : '',
  )
  const [webhookURL, setWebhookURL] = useState('')
  const [webhookSecret, setWebhookSecret] = useState('')
  const [webhookHeaders, setWebhookHeaders] = useState('')
  const [allowPrivate, setAllowPrivate] = useState(Boolean(channel?.config.allow_private))
  const [discordURL, setDiscordURL] = useState('')
  const [discordUsername, setDiscordUsername] = useState('')
  const [discordAvatar, setDiscordAvatar] = useState('')
  const [botToken, setBotToken] = useState('')
  const [chatID, setChatID] = useState(
    channel?.type === 'telegram' ? String(channel.config.chat_id ?? '') : '',
  )
  const [threadID, setThreadID] = useState(
    channel?.type === 'telegram' ? String(channel.config.thread_id ?? '') : '',
  )

  const config = useMemo((): Record<string, unknown> | null => {
    if (!replaceConfig) return null
    switch (type) {
      case 'email':
        return { recipients: parseRecipients(recipients) }
      case 'webhook': {
        const headers: Record<string, string> = {}
        for (const line of webhookHeaders.split('\n')) {
          const index = line.indexOf(':')
          if (index > 0) {
            headers[line.slice(0, index).trim()] = line.slice(index + 1).trim()
          }
        }
        return {
          url: webhookURL,
          ...(webhookSecret ? { secret: webhookSecret } : {}),
          ...(Object.keys(headers).length ? { headers } : {}),
          allow_private: allowPrivate,
        }
      }
      case 'discord':
        return {
          webhook_url: discordURL,
          ...(discordUsername ? { username: discordUsername } : {}),
          ...(discordAvatar ? { avatar_url: discordAvatar } : {}),
        }
      case 'telegram':
        return {
          bot_token: botToken,
          chat_id: chatID,
          ...(threadID ? { thread_id: threadID } : {}),
        }
    }
  }, [
    allowPrivate,
    botToken,
    chatID,
    discordAvatar,
    discordURL,
    discordUsername,
    recipients,
    replaceConfig,
    threadID,
    type,
    webhookHeaders,
    webhookSecret,
    webhookURL,
  ])

  const mutation = useMutation({
    mutationFn: () => {
      if (channel) {
        return api.updateChannel(channel.id, {
          name,
          ...(config ? { config } : {}),
          enabled,
          receive_system_alerts: systemAlerts,
        })
      }
      if (!config) throw new Error('Channel configuration is required.')
      return api.createChannel({
        name,
        type,
        config,
        enabled,
        receive_system_alerts: systemAlerts,
      })
    },
    onSuccess: onSaved,
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  const channels = useChannels()
  const channelCount = channels.data?.items.length ?? 0

  return (
    <Dialog
      open
      wide
      onClose={() => {
        if (!mutation.isPending) onClose()
      }}
      title={channel ? `Edit ${channel.name}` : 'Add delivery channel'}
      description={
        channel
          ? 'Secrets stay encrypted. Leave configuration unchanged unless you explicitly replace it.'
          : 'The channel is validated before it is stored. Secrets are encrypted at rest.'
      }
      footer={
        <>
          <SecondaryButton onClick={onClose} disabled={mutation.isPending}>
            Cancel
          </SecondaryButton>
          <PrimaryButton type="submit" onClick={() => mutation.mutate()} disabled={!name.trim() || mutation.isPending}>
            {mutation.isPending ? 'Saving…' : channel ? 'Save channel' : 'Add channel'}
          </PrimaryButton>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Channel name">
            <TextInput
              autoFocus
              value={name}
              onChange={(event) => setName(event.target.value)}
              placeholder="Ops email"
            />
          </FormField>
          <FormField label="Type">
            <select
              value={type}
              onChange={(event) => setType(event.target.value as ChannelType)}
              disabled={Boolean(channel)}
              className="h-9 w-full rounded-md border border-line bg-canvas px-2 text-xs text-ink disabled:cursor-not-allowed disabled:opacity-50"
            >
              <option value="email">Email</option>
              <option value="webhook">Webhook</option>
              <option value="discord">Discord</option>
              <option value="telegram">Telegram</option>
            </select>
          </FormField>
        </div>

        <div className="grid gap-3 rounded-md border border-line bg-surface-2/35 px-3 py-3 sm:grid-cols-2">
          <Checkbox
            checked={enabled}
            onChange={(event) => setEnabled(event.target.checked)}
            label="Enabled"
            hint="Disabled channels cancel pending deliveries."
          />
          <Checkbox
            checked={systemAlerts}
            onChange={(event) => setSystemAlerts(event.target.checked)}
            label="Receive system alerts"
            hint="Check failures and unknown expiry notices go to this channel."
          />
        </div>

        {channel ? (
          <Checkbox
            checked={replaceConfig}
            onChange={(event) => setReplaceConfig(event.target.checked)}
            label="Replace channel configuration"
            hint="Leave unchecked to rename or toggle this channel without touching its secret configuration."
          />
        ) : null}

        {channel && !replaceConfig ? (
          <div className="rounded-md border border-line bg-surface-2/35 px-3 py-2.5">
            <p className="text-[10px] font-semibold uppercase tracking-[0.08em] text-faint">Stored configuration</p>
            <p className="mt-1 text-[11px] text-muted">{summaryLine(channel)}</p>
          </div>
        ) : null}

        {replaceConfig ? (
          <div className="space-y-4 rounded-md border border-line px-3 py-3">
            {type === 'email' ? (
              <FormField label="Recipients" hint="One address per line, or separate with commas.">
                <Textarea
                  value={recipients}
                  onChange={(event) => setRecipients(event.target.value)}
                  placeholder="ops@example.com"
                />
              </FormField>
            ) : null}

            {type === 'webhook' ? (
              <>
                <FormField label="Webhook URL" hint="HTTPS is required unless private networks are allowed.">
                  <TextInput
                    type="url"
                    value={webhookURL}
                    onChange={(event) => setWebhookURL(event.target.value)}
                    placeholder="https://hooks.example.com/iwmmd"
                  />
                </FormField>
                <div className="grid gap-4 sm:grid-cols-2">
                  <FormField label="HMAC secret" hint="Optional. Sent as X-IWMMD-Signature.">
                    <TextInput
                      type="password"
                      value={webhookSecret}
                      onChange={(event) => setWebhookSecret(event.target.value)}
                      placeholder={channel ? 'Leave blank to keep stored secret' : 'Optional'}
                      autoComplete="new-password"
                    />
                  </FormField>
                  <div className="flex items-end pb-1">
                    <Checkbox
                      checked={allowPrivate}
                      onChange={(event) => setAllowPrivate(event.target.checked)}
                      label="Allow private network destinations"
                      hint="Only use this for trusted internal receivers."
                    />
                  </div>
                </div>
                <FormField label="Extra headers" hint="Optional. One header per line as Name: value.">
                  <Textarea
                    value={webhookHeaders}
                    onChange={(event) => setWebhookHeaders(event.target.value)}
                    placeholder="Authorization: Bearer token"
                  />
                </FormField>
              </>
            ) : null}

            {type === 'discord' ? (
              <>
                <FormField label="Discord webhook URL">
                  <TextInput
                    type="url"
                    value={discordURL}
                    onChange={(event) => setDiscordURL(event.target.value)}
                    placeholder="https://discord.com/api/webhooks/..."
                  />
                </FormField>
                <div className="grid gap-4 sm:grid-cols-2">
                  <FormField label="Username override">
                    <TextInput
                      value={discordUsername}
                      onChange={(event) => setDiscordUsername(event.target.value)}
                      placeholder="IWMMD"
                    />
                  </FormField>
                  <FormField label="Avatar URL">
                    <TextInput
                      type="url"
                      value={discordAvatar}
                      onChange={(event) => setDiscordAvatar(event.target.value)}
                      placeholder="https://example.com/avatar.png"
                    />
                  </FormField>
                </div>
              </>
            ) : null}

            {type === 'telegram' ? (
              <>
                <FormField
                  label="Bot token"
                  hint={channel ? 'Leave blank to keep the stored token.' : 'Create a bot with BotFather.'}
                >
                  <TextInput
                    type="password"
                    value={botToken}
                    onChange={(event) => setBotToken(event.target.value)}
                    placeholder={channel ? 'Leave blank to keep stored token' : '123456:ABC...'}
                    autoComplete="new-password"
                  />
                </FormField>
                <div className="grid gap-4 sm:grid-cols-2">
                  <FormField label="Chat ID">
                    <TextInput value={chatID} onChange={(event) => setChatID(event.target.value)} placeholder="-1001234567890" />
                  </FormField>
                  <FormField label="Topic / thread ID" hint="Optional forum topic.">
                    <TextInput value={threadID} onChange={(event) => setThreadID(event.target.value)} />
                  </FormField>
                </div>
              </>
            ) : null}
          </div>
        ) : null}

        {channelCount > 0 ? (
          <p className="text-[10px] leading-4 text-faint">
            Existing rules keep their channel selection even when this channel is disabled or muted.
          </p>
        ) : null}

        {mutation.error ? (
          <Banner tone="error" title="Channel could not be saved">
            {errorMessage(mutation.error)}
          </Banner>
        ) : null}
      </form>
    </Dialog>
  )
}

function MuteChannelDialog({
  channel,
  onClose,
  onSaved,
}: {
  channel: Channel
  onClose: () => void
  onSaved: () => void
}) {
  // The default window is relative to the moment this dialog mounts.
  // oxlint-disable-next-line react/purity
  const [until, setUntil] = useState(toDateTimeLocal(new Date(Date.now() + 24 * 60 * 60 * 1000).toISOString()))
  const [reason, setReason] = useState(channel.mute_reason ?? '')
  const mute = useMutation({
    mutationFn: () => {
      const at = fromDateTimeLocal(until)
      if (!at) throw new Error('Choose a mute end time.')
      return api.muteChannel(channel.id, { until: at, reason: reason || undefined })
    },
    onSuccess: onSaved,
  })
  const unmute = useMutation({
    mutationFn: () => api.unmuteChannel(channel.id),
    onSuccess: onSaved,
  })

  return (
    <Dialog
      open
      onClose={onClose}
      title={`Mute ${channel.name}`}
      description="All pending and future notifications through this channel are held until the mute ends."
      footer={
        <>
          {channel.muted_until ? (
            <SecondaryButton onClick={() => unmute.mutate()} disabled={unmute.isPending}>
              <PlayIcon className="size-3.5" />
              Unmute now
            </SecondaryButton>
          ) : null}
          <div className="flex-1" />
          <SecondaryButton onClick={onClose}>Cancel</SecondaryButton>
          <PrimaryButton onClick={() => mute.mutate()} disabled={mute.isPending}>
            <PauseIcon className="size-3.5" />
            {mute.isPending ? 'Saving…' : 'Mute channel'}
          </PrimaryButton>
        </>
      }
    >
      <div className="space-y-4">
        {channel.muted_until ? (
          <Banner tone="warn" title="Currently muted">
            Until {formatDateTime(channel.muted_until)}
            {channel.mute_reason ? ` · ${channel.mute_reason}` : ''}
          </Banner>
        ) : null}
        <FormField label="Mute until">
          <TextInput type="datetime-local" value={until} onChange={(event) => setUntil(event.target.value)} />
        </FormField>
        <FormField label="Reason">
          <TextInput value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Maintenance window" />
        </FormField>
        {mute.error ? <Banner tone="error" title="Mute failed">{errorMessage(mute.error)}</Banner> : null}
        {unmute.error ? <Banner tone="error" title="Unmute failed">{errorMessage(unmute.error)}</Banner> : null}
      </div>
    </Dialog>
  )
}

function ArchiveChannelDialog({
  channel,
  onClose,
  onArchived,
}: {
  channel: Channel
  onClose: () => void
  onArchived: () => void
}) {
  const mutation = useMutation({
    mutationFn: () => api.archiveChannel(channel.id),
    onSuccess: onArchived,
  })
  return (
    <Dialog
      open
      onClose={onClose}
      title={`Archive ${channel.name}`}
      description="Pending notifications for this channel are canceled. Rules that reference it keep their history."
      footer={
        <>
          <SecondaryButton onClick={onClose}>Cancel</SecondaryButton>
          <DangerButton onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            <TrashIcon className="size-3.5" />
            {mutation.isPending ? 'Archiving…' : 'Archive channel'}
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
          This channel will stop receiving alerts. {titleCase(channel.type)} configuration stays encrypted in the
          database.
        </p>
      )}
    </Dialog>
  )
}
