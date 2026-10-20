import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import {
  api,
  useChecks,
  useDomain,
  useDomains,
  useIncidents,
  useMe,
  useNotifications,
  useRules,
  type Domain,
  type Rule,
} from '../lib/api'
import {
  daysLabel,
  domainTone,
  expiryLabel,
  formatDate,
  formatDateTime,
  fromDateTimeLocal,
  offsetLabel,
  relativeTime,
  titleCase,
  toDateTimeLocal,
} from '../lib/format'
import {
  DownloadIcon,
  GlobeIcon,
  PauseIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  RefreshIcon,
  SearchIcon,
  TrashIcon,
  UploadIcon,
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
  Select,
  Spinner,
  StatusPill,
  Textarea,
  TextInput,
} from '../components/ui'

function errorMessage(error: unknown): string {
  if (error instanceof Error) return error.message
  return 'The request could not be completed.'
}

function mutateRefresh(client: ReturnType<typeof useQueryClient>) {
  void client.invalidateQueries({ queryKey: ['domains'] })
  void client.invalidateQueries({ queryKey: ['stats'] })
  void client.invalidateQueries({ queryKey: ['domain'] })
  void client.invalidateQueries({ queryKey: ['checks'] })
  void client.invalidateQueries({ queryKey: ['notifications'] })
}

export function DomainsPage() {
  const client = useQueryClient()
  const me = useMe()
  const canWrite = me.data?.user?.role === 'member' || me.data?.user?.role === 'admin'

  const [search, setSearch] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [state, setState] = useState('')
  const [expiry, setExpiry] = useState('')
  const [offset, setOffset] = useState(0)
  const [addOpen, setAddOpen] = useState(false)
  const [importOpen, setImportOpen] = useState(false)
  const [selectedID, setSelectedID] = useState<string | null>(null)
  const [editDomain, setEditDomain] = useState<Domain | null>(null)
  const [muteDomain, setMuteDomain] = useState<Domain | null>(null)
  const [archiveDomainTarget, setArchiveDomainTarget] = useState<Domain | null>(null)

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setDebouncedSearch(search)
      setOffset(0)
    }, 250)
    return () => window.clearTimeout(timer)
  }, [search])

  const domains = useDomains({
    search: debouncedSearch || undefined,
    state: state || undefined,
    expiry: expiry || undefined,
    limit: 25,
    offset,
  })

  const refresh = () => {
    void client.invalidateQueries()
  }

  const total = domains.data?.total ?? 0
  const items = domains.data?.items ?? []
  const hasFilters = Boolean(search || state || expiry)

  return (
    <div className="space-y-5">
      <PageHeader
        title="Domains"
        description="The names IWMMD is responsible for. Checks run on a schedule and on demand."
        actions={
          <>
            {canWrite ? (
              <SecondaryButton onClick={() => setImportOpen(true)} title="Import domains from CSV">
                <UploadIcon className="size-3.5" />
                Import
              </SecondaryButton>
            ) : null}
            <PrimaryButton
              disabled={!canWrite}
              title={canWrite ? 'Add a domain' : 'Your role cannot add domains'}
              onClick={() => setAddOpen(true)}
            >
              <PlusIcon className="size-3.5" />
              Add domain
            </PrimaryButton>
          </>
        }
      />

      {domains.error ? (
        <Banner tone="error" title="Could not load domains">
          {errorMessage(domains.error)}
        </Banner>
      ) : null}

      <Panel
        title="Watchlist"
        description="Search by name or registrar, and narrow the list by state or expiry."
        action={
          <div className="flex items-center gap-2">
            <StatusPill tone="neutral" dot={false}>
              {total} {total === 1 ? 'domain' : 'domains'}
            </StatusPill>
            <button
              type="button"
              onClick={refresh}
              className="grid size-8 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-ink"
              title="Refresh domains"
              aria-label="Refresh domains"
            >
              <RefreshIcon className={`size-3.5 ${domains.isFetching ? 'animate-spin' : ''}`} />
            </button>
          </div>
        }
      >
        <div className="flex flex-col gap-2 border-b border-line px-4 py-3 sm:flex-row sm:items-center sm:px-5">
          <div className="relative min-w-0 flex-1">
            <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-faint" />
            <TextInput
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="Search domains or registrars"
              className="pl-8"
              aria-label="Search domains"
            />
          </div>
          <div className="flex gap-2">
            <Select
              value={state}
              onChange={(event) => {
                setState(event.target.value)
                setOffset(0)
              }}
              aria-label="Filter by state"
              className="sm:w-36"
            >
              <option value="">All states</option>
              <option value="active">Active</option>
              <option value="paused">Paused</option>
              <option value="archived">Archived</option>
            </Select>
            <Select
              value={expiry}
              onChange={(event) => {
                setExpiry(event.target.value)
                setOffset(0)
              }}
              aria-label="Filter by expiry"
              className="sm:w-40"
            >
              <option value="">Any expiry</option>
              <option value="7d">Within 7 days</option>
              <option value="30d">Within 30 days</option>
              <option value="90d">Within 90 days</option>
              <option value="expired">Expired</option>
              <option value="unknown">Unknown</option>
            </Select>
          </div>
        </div>

        {domains.isPending ? (
          <Spinner label="Loading domains" />
        ) : items.length === 0 ? (
          <div className="px-4 py-5 sm:px-5">
            <EmptyState
              icon={GlobeIcon}
              title={hasFilters ? 'No domains match these filters' : 'No domains under watch'}
              description={
                hasFilters
                  ? 'Clear the search and filters, or add another domain.'
                  : 'Add a domain and IWMMD will queue the first RDAP check immediately.'
              }
              action={
                hasFilters ? (
                  <SecondaryButton
                    onClick={() => {
                      setSearch('')
                      setState('')
                      setExpiry('')
                      setOffset(0)
                    }}
                  >
                    Clear filters
                  </SecondaryButton>
                ) : canWrite ? (
                  <PrimaryButton onClick={() => setAddOpen(true)}>
                    <PlusIcon className="size-3.5" />
                    Add domain
                  </PrimaryButton>
                ) : undefined
              }
            />
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full min-w-[52rem] border-collapse text-left">
              <thead>
                <tr className="border-b border-line">
                  {['Domain', 'Registrar', 'Expiry', 'Next check', 'State', ''].map((heading, index) => (
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
                {items.map((domain) => (
                  <tr key={domain.id} className="group transition-colors hover:bg-surface-2/45">
                    <td className="px-4 py-3 sm:px-5">
                      <button
                        type="button"
                        onClick={() => setSelectedID(domain.id)}
                        className="text-left text-xs font-semibold text-ink transition-colors hover:text-accent"
                      >
                        {domain.name}
                      </button>
                      <p className="mt-0.5 text-[10px] text-faint">
                        {domain.muted_until ? `Muted until ${formatDateTime(domain.muted_until)}` : domain.name_ace}
                      </p>
                    </td>
                    <td className="px-4 py-3 text-[11px] text-muted sm:px-5">
                      {domain.registrar || <span className="text-faint">Unknown</span>}
                    </td>
                    <td className="px-4 py-3 sm:px-5">
                      <p className="text-[11px] text-ink">{expiryLabel(domain)}</p>
                      <p className="mt-0.5 text-[10px] text-faint">
                        Confidence {domain.expiry_confidence}
                      </p>
                    </td>
                    <td className="px-4 py-3 sm:px-5">
                      <p className="text-[11px] text-muted">
                        {domain.state === 'paused' ? 'Paused' : relativeTime(domain.next_check_at)}
                      </p>
                      <p className="mt-0.5 text-[10px] text-faint">
                        Last {relativeTime(domain.last_checked_at)}
                      </p>
                    </td>
                    <td className="px-4 py-3 sm:px-5">
                      <StatusPill tone={domain.state === 'archived' ? 'neutral' : domainTone(domain)}>
                        {domain.state === 'active'
                          ? domain.last_check_status === 'error'
                            ? 'check failing'
                            : daysLabel(domain.days_left)
                          : domain.state}
                      </StatusPill>
                    </td>
                    <td className="px-4 py-3 text-right sm:px-5">
                      <div className="flex items-center justify-end gap-1">
                        <button
                          type="button"
                          onClick={() => setSelectedID(domain.id)}
                          className="grid size-7 place-items-center rounded-md text-muted transition-colors hover:bg-surface-2 hover:text-ink"
                          title="View domain"
                          aria-label={`View ${domain.name}`}
                        >
                          <SearchIcon className="size-3.5" />
                        </button>
                        {canWrite ? (
                          <button
                            type="button"
                            onClick={() => setEditDomain(domain)}
                            className="grid size-7 place-items-center rounded-md text-muted transition-colors hover:bg-surface-2 hover:text-ink"
                            title="Edit domain"
                            aria-label={`Edit ${domain.name}`}
                          >
                            <PencilIcon className="size-3.5" />
                          </button>
                        ) : null}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {total > 25 ? (
          <div className="flex items-center justify-between border-t border-line bg-surface-2/35 px-4 py-2.5 text-[10px] text-muted sm:px-5">
            <span>
              Showing {offset + 1}-{Math.min(offset + 25, total)} of {total}
            </span>
            <div className="flex gap-2">
              <SecondaryButton
                disabled={offset === 0}
                onClick={() => setOffset(Math.max(0, offset - 25))}
                title="Previous page"
              >
                Previous
              </SecondaryButton>
              <SecondaryButton
                disabled={offset + 25 >= total}
                onClick={() => setOffset(offset + 25)}
                title="Next page"
              >
                Next
              </SecondaryButton>
            </div>
          </div>
        ) : null}
      </Panel>

      <AddDomainDialog open={addOpen} onClose={() => setAddOpen(false)} />
      <ImportDomainsDialog open={importOpen} onClose={() => setImportOpen(false)} />
      {editDomain ? (
        <EditDomainDialog
          key={editDomain.id}
          domain={editDomain}
          onClose={() => setEditDomain(null)}
          onMute={() => {
            setMuteDomain(editDomain)
            setEditDomain(null)
          }}
          onArchive={() => {
            setArchiveDomainTarget(editDomain)
            setEditDomain(null)
          }}
        />
      ) : null}
      {muteDomain ? (
        <MuteDomainDialog
          key={muteDomain.id}
          domain={muteDomain}
          onClose={() => setMuteDomain(null)}
        />
      ) : null}
      {archiveDomainTarget ? (
        <ArchiveDomainDialog
          key={archiveDomainTarget.id}
          domain={archiveDomainTarget}
          onClose={() => setArchiveDomainTarget(null)}
        />
      ) : null}
      <DomainDetailDialog
        domainID={selectedID}
        canWrite={canWrite}
        onClose={() => setSelectedID(null)}
        onEdit={(domain) => {
          setSelectedID(null)
          setEditDomain(domain)
        }}
        onMute={(domain) => {
          setSelectedID(null)
          setMuteDomain(domain)
        }}
        onArchive={(domain) => {
          setSelectedID(null)
          setArchiveDomainTarget(domain)
        }}
      />
    </div>
  )
}

function AddDomainDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [notes, setNotes] = useState('')
  const mutation = useMutation({
    mutationFn: () => api.createDomain({ name, notes: notes || undefined }),
    onSuccess: () => {
      mutateRefresh(client)
      setName('')
      setNotes('')
      onClose()
    },
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    if (name.trim()) mutation.mutate()
  }

  return (
    <Dialog
      open={open}
      onClose={() => {
        if (!mutation.isPending) onClose()
      }}
      title="Add domain"
      description="The first check runs immediately. Internationalized names are normalized to IDNA."
      footer={
        <>
          <SecondaryButton onClick={onClose} disabled={mutation.isPending}>
            Cancel
          </SecondaryButton>
          <PrimaryButton type="submit" onClick={() => mutation.mutate()} disabled={!name.trim() || mutation.isPending}>
            Add domain
          </PrimaryButton>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        <FormField label="Domain name" hint="For example example.com or bücher.example. Do not include a URL path.">
          <TextInput
            autoFocus
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder="example.com"
            autoComplete="off"
          />
        </FormField>
        <FormField label="Notes" hint="Optional context for the team, such as the owning project.">
          <Textarea value={notes} onChange={(event) => setNotes(event.target.value)} placeholder="Optional notes" />
        </FormField>
        {mutation.error ? (
          <Banner tone="error" title="Domain could not be added">
            {errorMessage(mutation.error)}
          </Banner>
        ) : null}
      </form>
    </Dialog>
  )
}

function ImportDomainsDialog({ open, onClose }: { open: boolean; onClose: () => void }) {
  const client = useQueryClient()
  const [file, setFile] = useState<File | null>(null)
  const mutation = useMutation({
    mutationFn: () => api.importDomains(file as File),
    onSuccess: () => {
      mutateRefresh(client)
    },
  })

  return (
    <Dialog
      open={open}
      onClose={() => {
        if (!mutation.isPending) {
          setFile(null)
          mutation.reset()
          onClose()
        }
      }}
      title="Import domains"
      description="Upload a CSV with one domain per row. An optional second column becomes notes."
      footer={
        <>
          <SecondaryButton
            onClick={() => {
              setFile(null)
              mutation.reset()
              onClose()
            }}
            disabled={mutation.isPending}
          >
            Close
          </SecondaryButton>
          <PrimaryButton
            disabled={!file || mutation.isPending}
            onClick={() => file && mutation.mutate()}
          >
            <UploadIcon className="size-3.5" />
            {mutation.isPending ? 'Importing…' : 'Import CSV'}
          </PrimaryButton>
        </>
      }
    >
      <div className="space-y-4">
        <label className="flex cursor-pointer flex-col items-center justify-center rounded-md border border-dashed border-line-strong bg-surface-2/45 px-4 py-8 text-center">
          <UploadIcon className="size-6 text-muted" />
          <span className="mt-2 text-xs font-semibold text-ink">
            {file ? file.name : 'Choose a CSV file'}
          </span>
          <span className="mt-1 text-[10px] text-faint">Maximum 2 MB. First row may be a header.</span>
          <input
            type="file"
            accept=".csv,text/csv"
            className="sr-only"
            onChange={(event) => {
              mutation.reset()
              setFile(event.target.files?.[0] ?? null)
            }}
          />
        </label>
        {mutation.data ? (
          <Banner tone={mutation.data.failed.length ? 'warn' : 'ok'} title={`${mutation.data.imported} domains imported`}>
            {mutation.data.failed.length
              ? `${mutation.data.failed.length} rows failed. ${mutation.data.failed
                  .slice(0, 5)
                  .map((item) => `${item.domain}: ${item.error}`)
                  .join(' · ')}`
              : 'Every row was accepted.'}
          </Banner>
        ) : null}
        {mutation.error ? (
          <Banner tone="error" title="Import failed">
            {errorMessage(mutation.error)}
          </Banner>
        ) : null}
      </div>
    </Dialog>
  )
}

function EditDomainDialog({
  domain,
  onClose,
  onMute,
  onArchive,
}: {
  domain: Domain
  onClose: () => void
  onMute: () => void
  onArchive: () => void
}) {
  const client = useQueryClient()
  const [registrar, setRegistrar] = useState(domain.registrar ?? '')
  const [notes, setNotes] = useState(domain.notes ?? '')
  const [inherit, setInherit] = useState(domain.inherit_global_alerts)
  const [interval, setInterval] = useState(String(domain.check_interval_seconds))
  const [state, setState] = useState(domain.state === 'paused' ? 'paused' : 'active')

  const mutation = useMutation({
    mutationFn: () =>
      api.updateDomain(domain.id, {
        version: domain.version,
        registrar,
        notes,
        inherit_global_alerts: inherit,
        check_interval_seconds: Number(interval),
        state,
      }),
    onSuccess: () => {
      mutateRefresh(client)
      onClose()
    },
  })

  const submit = (event: FormEvent) => {
    event.preventDefault()
    mutation.mutate()
  }

  return (
    <Dialog
      open
      wide
      onClose={() => {
        if (!mutation.isPending) onClose()
      }}
      title={`Edit ${domain.name}`}
      description="Changes to alert inheritance and state are reconciled against pending notifications."
      footer={
        <>
          <DangerButton onClick={onArchive} title="Archive domain">
            <TrashIcon className="size-3.5" />
            Archive
          </DangerButton>
          <div className="flex-1" />
          <SecondaryButton onClick={onMute}>
            <PauseIcon className="size-3.5" />
            Mute
          </SecondaryButton>
          <SecondaryButton onClick={onClose} disabled={mutation.isPending}>
            Cancel
          </SecondaryButton>
          <PrimaryButton type="submit" onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            Save changes
          </PrimaryButton>
        </>
      }
    >
      <form onSubmit={submit} className="space-y-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Registrar" hint="Leave blank to keep the registrar returned by the registry.">
            <TextInput value={registrar} onChange={(event) => setRegistrar(event.target.value)} />
          </FormField>
          <FormField label="Check interval" hint="Seconds between scheduled checks. Minimum 300.">
            <TextInput
              type="number"
              min={300}
              step={300}
              value={interval}
              onChange={(event) => setInterval(event.target.value)}
            />
          </FormField>
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField label="Domain state">
            <Select value={state} onChange={(event) => setState(event.target.value)}>
              <option value="active">Active</option>
              <option value="paused">Paused</option>
            </Select>
          </FormField>
          <div className="flex items-end pb-1">
            <Checkbox
              checked={inherit}
              onChange={(event) => setInherit(event.target.checked)}
              label="Inherit global alert rules"
              hint="When disabled, only rules scoped to this domain apply."
            />
          </div>
        </div>
        <FormField label="Notes">
          <Textarea value={notes} onChange={(event) => setNotes(event.target.value)} />
        </FormField>
        {mutation.error ? (
          <Banner tone="error" title="Changes could not be saved">
            {errorMessage(mutation.error)}
          </Banner>
        ) : null}
      </form>
    </Dialog>
  )
}

function MuteDomainDialog({ domain, onClose }: { domain: Domain; onClose: () => void }) {
  const client = useQueryClient()
  // The default window is relative to the moment this dialog mounts.
  // oxlint-disable-next-line react/purity
  const [until, setUntil] = useState(toDateTimeLocal(new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString()))
  const [reason, setReason] = useState(domain.mute_reason ?? '')
  const mute = useMutation({
    mutationFn: () => {
      const at = fromDateTimeLocal(until)
      if (!at) throw new Error('Choose a mute end time.')
      return api.muteDomain(domain.id, { until: at, reason: reason || undefined })
    },
    onSuccess: () => {
      mutateRefresh(client)
      onClose()
    },
  })
  const unmute = useMutation({
    mutationFn: () => api.unmuteDomain(domain.id),
    onSuccess: () => {
      mutateRefresh(client)
      onClose()
    },
  })

  return (
    <Dialog
      open
      onClose={onClose}
      title={`Mute ${domain.name}`}
      description="Expiry alerts are held until this time. Checks continue and system alerts are unaffected."
      footer={
        <>
          {domain.muted_until ? (
            <SecondaryButton onClick={() => unmute.mutate()} disabled={unmute.isPending}>
              <PlayIcon className="size-3.5" />
              Unmute now
            </SecondaryButton>
          ) : null}
          <div className="flex-1" />
          <SecondaryButton onClick={onClose}>Cancel</SecondaryButton>
          <PrimaryButton onClick={() => mute.mutate()} disabled={mute.isPending}>
            <PauseIcon className="size-3.5" />
            {mute.isPending ? 'Saving…' : 'Mute alerts'}
          </PrimaryButton>
        </>
      }
    >
      <div className="space-y-4">
        {domain.muted_until ? (
          <Banner tone="warn" title="Currently muted">
            Until {formatDateTime(domain.muted_until)}
            {domain.mute_reason ? ` · ${domain.mute_reason}` : ''}
          </Banner>
        ) : null}
        <FormField label="Mute until">
          <TextInput type="datetime-local" value={until} onChange={(event) => setUntil(event.target.value)} />
        </FormField>
        <FormField label="Reason" hint="Optional context for the team.">
          <TextInput value={reason} onChange={(event) => setReason(event.target.value)} placeholder="Renewal in progress" />
        </FormField>
        {mute.error ? (
          <Banner tone="error" title="Mute could not be applied">
            {errorMessage(mute.error)}
          </Banner>
        ) : null}
        {unmute.error ? (
          <Banner tone="error" title="Unmute failed">
            {errorMessage(unmute.error)}
          </Banner>
        ) : null}
      </div>
    </Dialog>
  )
}

function ArchiveDomainDialog({ domain, onClose }: { domain: Domain; onClose: () => void }) {
  const client = useQueryClient()
  const mutation = useMutation({
    mutationFn: () => api.archiveDomain(domain.id),
    onSuccess: () => {
      mutateRefresh(client)
      onClose()
    },
  })
  return (
    <Dialog
      open
      onClose={onClose}
      title={`Archive ${domain.name}`}
      description="Pending checks and expiry alerts are canceled. The domain remains in history."
      footer={
        <>
          <SecondaryButton onClick={onClose}>Cancel</SecondaryButton>
          <DangerButton onClick={() => mutation.mutate()} disabled={mutation.isPending}>
            <TrashIcon className="size-3.5" />
            {mutation.isPending ? 'Archiving…' : 'Archive domain'}
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
          This does not delete check history. Add the domain again later if it returns to the watchlist.
        </p>
      )}
    </Dialog>
  )
}

function DomainDetailDialog({
  domainID,
  canWrite,
  onClose,
  onEdit,
  onMute,
  onArchive,
}: {
  domainID: string | null
  canWrite: boolean
  onClose: () => void
  onEdit: (domain: Domain) => void
  onMute: (domain: Domain) => void
  onArchive: (domain: Domain) => void
}) {
  const client = useQueryClient()
  const domain = useDomain(domainID)
  const checks = useChecks(domainID)
  const notifications = useNotifications({ domain_id: domainID ?? undefined, limit: 8 })
  const incidents = useIncidents({ domain_id: domainID ?? undefined, limit: 8 })
  const rules = useRules({ domain_id: domainID ?? undefined })
  const checkNow = useMutation({
    mutationFn: () => api.checkDomain(domainID as string),
    onSuccess: () => mutateRefresh(client),
  })

  if (!domainID) return null
  const value = domain.data

  return (
    <Dialog
      open
      wide
      onClose={onClose}
      title={value?.name ?? 'Domain detail'}
      description={value ? `${value.name_ace} · version ${value.version}` : 'Loading domain…'}
      footer={
        value ? (
          <>
            <DangerButton onClick={() => onArchive(value)}>
              <TrashIcon className="size-3.5" />
              Archive
            </DangerButton>
            <div className="flex-1" />
            <SecondaryButton onClick={() => checkNow.mutate()} disabled={checkNow.isPending || value.state !== 'active'}>
              <PlayIcon className="size-3.5" />
              {checkNow.isPending ? 'Queueing…' : 'Check now'}
            </SecondaryButton>
            <SecondaryButton onClick={() => onMute(value)}>
              <PauseIcon className="size-3.5" />
              Mute
            </SecondaryButton>
            {canWrite ? (
              <PrimaryButton onClick={() => onEdit(value)}>
                <PencilIcon className="size-3.5" />
                Edit
              </PrimaryButton>
            ) : null}
          </>
        ) : null
      }
    >
      {domain.isPending ? <Spinner label="Loading domain" /> : null}
      {domain.error ? (
        <Banner tone="error" title="Domain could not be loaded">
          {errorMessage(domain.error)}
        </Banner>
      ) : null}
      {value ? (
        <div className="space-y-5">
          <div className="grid gap-2 sm:grid-cols-3">
            <Detail label="Expiry" value={expiryLabel(value)} />
            <Detail label="State" value={titleCase(value.state)} />
            <Detail label="Last check" value={formatDateTime(value.last_checked_at)} />
            <Detail label="Registrar" value={value.registrar || 'Unknown'} />
            <Detail label="Next check" value={formatDateTime(value.next_check_at, 'Not scheduled')} />
            <Detail label="Mute" value={value.muted_until ? `Until ${formatDateTime(value.muted_until)}` : 'Not muted'} />
          </div>
          {value.notes ? (
            <div className="rounded-md border border-line bg-surface-2/45 px-3 py-2.5 text-[11px] leading-5 text-muted">
              {value.notes}
            </div>
          ) : null}
          {value.last_check_error ? (
            <Banner tone="warn" title="Latest check error">
              {value.last_check_error}
            </Banner>
          ) : null}

          <DetailSection title="Check history" description="Raw responses remain available until retention removes them.">
            {checks.isPending ? (
              <Spinner label="Loading checks" />
            ) : (checks.data?.items.length ?? 0) === 0 ? (
              <p className="text-[11px] text-muted">No checks recorded yet.</p>
            ) : (
              <div className="overflow-x-auto rounded-md border border-line">
                <table className="w-full min-w-[36rem] border-collapse text-left">
                  <thead className="bg-surface-2/55">
                    <tr>
                      {['Started', 'Source', 'Status', 'Expiry', 'Duration', 'Raw'].map((heading) => (
                        <th key={heading} className="px-2.5 py-2 text-[10px] font-semibold text-faint">
                          {heading}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {checks.data?.items.map((check) => (
                      <tr key={check.id}>
                        <td className="px-2.5 py-2 text-[10px] text-muted">{formatDateTime(check.started_at)}</td>
                        <td className="px-2.5 py-2 text-[10px] uppercase text-muted">{check.source}</td>
                        <td className="px-2.5 py-2">
                          <StatusPill tone={check.status === 'ok' ? 'ok' : 'error'} dot={false}>
                            {check.status}
                          </StatusPill>
                        </td>
                        <td className="px-2.5 py-2 text-[10px] text-muted">{formatDate(check.expires_at, 'Unknown')}</td>
                        <td className="px-2.5 py-2 text-[10px] text-muted">{check.duration_ms} ms</td>
                        <td className="px-2.5 py-2 text-right">
                          <a
                            href={api.rawCheckURL(value.id, check.id)}
                            target="_blank"
                            rel="noreferrer"
                            className="inline-flex items-center gap-1 text-[10px] font-semibold text-accent hover:underline"
                          >
                            <DownloadIcon className="size-3" />
                            Open
                          </a>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </DetailSection>

          <DetailSection title="Domain rules" description="Rules scoped to this domain. Global rules apply unless inheritance is disabled.">
            {(rules.data?.items.length ?? 0) === 0 ? (
              <p className="text-[11px] text-muted">
                No domain-specific rules. {value.inherit_global_alerts ? 'Global rules are inherited.' : 'Global rules are ignored for this domain.'}
              </p>
            ) : (
              <RuleList rules={rules.data?.items ?? []} />
            )}
          </DetailSection>

          <div className="grid gap-4 sm:grid-cols-2">
            <DetailSection title="Open incidents" description="Check failures and unknown expiry.">
              {incidents.data?.items.length ? (
                <ul className="space-y-2">
                  {incidents.data.items.map((incident) => (
                    <li key={incident.id} className="rounded-md border border-line px-2.5 py-2 text-[10px]">
                      <div className="flex items-center justify-between gap-2">
                        <span className="font-semibold text-ink">{titleCase(incident.kind)}</span>
                        <StatusPill tone={incident.state === 'open' ? 'warn' : 'ok'} dot={false}>
                          {incident.state}
                        </StatusPill>
                      </div>
                      <p className="mt-1 text-faint">Opened {relativeTime(incident.opened_at)}</p>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-[11px] text-muted">No incidents recorded.</p>
              )}
            </DetailSection>
            <DetailSection title="Recent notifications" description="Latest outbox entries for this domain.">
              {notifications.data?.items.length ? (
                <ul className="space-y-2">
                  {notifications.data.items.map((item) => (
                    <li key={item.id} className="rounded-md border border-line px-2.5 py-2 text-[10px]">
                      <div className="flex items-center justify-between gap-2">
                        <span className="truncate text-muted">{item.channel_name || item.kind}</span>
                        <StatusPill
                          tone={
                            item.state === 'sent' ? 'ok' : item.state === 'failed' ? 'error' : 'neutral'
                          }
                          dot={false}
                        >
                          {item.state}
                        </StatusPill>
                      </div>
                      <p className="mt-1 text-faint">{formatDateTime(item.scheduled_at)}</p>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-[11px] text-muted">No notifications yet.</p>
              )}
            </DetailSection>
          </div>
        </div>
      ) : null}
    </Dialog>
  )
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border border-line bg-surface-2/40 px-2.5 py-2">
      <p className="text-[9px] font-semibold uppercase tracking-[0.08em] text-faint">{label}</p>
      <p className="mt-1 text-[11px] font-medium text-ink">{value}</p>
    </div>
  )
}

function DetailSection({
  title,
  description,
  children,
}: {
  title: string
  description: string
  children: ReactNode
}) {
  return (
    <section>
      <div className="mb-2">
        <h3 className="text-[11px] font-semibold text-ink">{title}</h3>
        <p className="mt-0.5 text-[10px] leading-4 text-faint">{description}</p>
      </div>
      {children}
    </section>
  )
}

function RuleList({ rules }: { rules: Rule[] }) {
  return (
    <ul className="divide-y divide-line rounded-md border border-line">
      {rules.map((rule) => (
        <li key={rule.id} className="flex flex-wrap items-center justify-between gap-2 px-2.5 py-2">
          <div>
            <p className="text-[11px] font-semibold text-ink">{rule.name}</p>
            <p className="mt-0.5 text-[10px] text-faint">
              {offsetLabel(rule)} at {rule.send_time_local}
            </p>
          </div>
          <StatusPill tone={rule.enabled ? 'ok' : 'neutral'} dot={false}>
            {rule.enabled ? 'enabled' : 'disabled'}
          </StatusPill>
        </li>
      ))}
    </ul>
  )
}
