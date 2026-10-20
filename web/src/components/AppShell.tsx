import { Link, Outlet, useRouterState } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useEffect, useState, type ComponentType, type SVGProps } from 'react'
import { api, setCSRFToken, useMe, useMeta, type User } from '../lib/api'
import { formatDateTime, formatVersion, titleCase } from '../lib/format'
import { ThemeControl } from './ThemeControl'
import { Banner, Spinner, StatusPill } from './ui'
import {
  BellIcon,
  AlertIcon,
  GlobeIcon,
  LogOutIcon,
  Mark,
  OverviewIcon,
  RadioIcon,
  ShieldIcon,
  SlidersIcon,
} from './icons'

type NavItem = {
  to: string
  label: string
  icon: ComponentType<SVGProps<SVGSVGElement>>
}

const navItems: NavItem[] = [
  { to: '/', label: 'Overview', icon: OverviewIcon },
  { to: '/domains', label: 'Domains', icon: GlobeIcon },
  { to: '/alerts', label: 'Alerts', icon: BellIcon },
  { to: '/channels', label: 'Channels', icon: RadioIcon },
  { to: '/incidents', label: 'Incidents', icon: AlertIcon },
  { to: '/settings', label: 'Settings', icon: SlidersIcon },
]

const pageDetails: Record<string, { title: string; detail: string }> = {
  '/': { title: 'Overview', detail: 'Watchlist health' },
  '/domains': { title: 'Domains', detail: 'Managed names' },
  '/alerts': { title: 'Alerts', detail: 'Expiry offsets' },
  '/channels': { title: 'Channels', detail: 'Delivery routes' },
  '/incidents': { title: 'Incidents', detail: 'Delivery and check health' },
  '/settings': { title: 'Settings', detail: 'System configuration' },
}

function NavLink({ item, mobile = false }: { item: NavItem; mobile?: boolean }) {
  const Icon = item.icon
  if (mobile) {
    return (
      <Link
        to={item.to}
        activeOptions={{ exact: item.to === '/' }}
        className="flex shrink-0 items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors"
        activeProps={{ className: 'bg-accent-soft text-ink' }}
        inactiveProps={{ className: 'text-muted hover:bg-surface-2 hover:text-ink' }}
      >
        <Icon className="size-3.5" />
        {item.label}
      </Link>
    )
  }
  return (
    <Link
      to={item.to}
      activeOptions={{ exact: item.to === '/' }}
      className="flex items-center gap-3 rounded-md px-3 py-2 text-[13px] font-medium transition-colors"
      activeProps={{ className: 'bg-accent-soft text-ink' }}
      inactiveProps={{ className: 'text-muted hover:bg-surface-2 hover:text-ink' }}
    >
      <Icon className="size-4 shrink-0" />
      {item.label}
    </Link>
  )
}

function initials(user: User) {
  const source = user.display_name || user.email || '?'
  return source
    .split(/[\s@._-]+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part.charAt(0).toUpperCase())
    .join('')
}

function CenteredState({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="grid min-h-dvh place-items-center bg-canvas px-5 text-ink">
      <div className="w-full max-w-md rounded-[10px] border border-line bg-surface p-6 shadow-sm">
        <Mark className="size-9 text-accent" />
        <h1 className="mt-4 text-lg font-semibold tracking-[-0.03em]">{title}</h1>
        <div className="mt-2 text-xs leading-5 text-muted">{children}</div>
      </div>
    </div>
  )
}

function Shell({ user }: { user: User }) {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const meta = useMeta()
  const queryClient = useQueryClient()
  const [signingOut, setSigningOut] = useState(false)
  const demoMode = meta.data?.demo_mode === true
  const page = pageDetails[pathname] ?? { title: 'Not found', detail: 'Unknown route' }

  const signOut = async () => {
    setSigningOut(true)
    try {
      await api.logout()
    } catch {
      // The local session is cleared even if the server cannot be reached.
    } finally {
      setCSRFToken(null)
      queryClient.clear()
      window.location.assign('/login')
    }
  }

  return (
    <div className="min-h-dvh bg-canvas text-ink">
      <aside className="fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r border-line bg-surface lg:flex">
        <Link to="/" className="flex items-center gap-3 px-5 py-5" aria-label="IWMMD overview">
          <Mark className="size-8 text-accent" />
          <span>
            <span className="block text-[15px] font-semibold tracking-[-0.03em] text-ink">IWMMD</span>
            <span className="block text-[10px] font-medium tracking-[0.12em] text-faint">DOMAIN WATCH</span>
          </span>
        </Link>

        <div className="mx-5 h-px bg-line" />

        <nav className="flex-1 px-2 py-4" aria-label="Primary">
          <div className="space-y-0.5">
            {navItems.map((item) => (
              <NavLink key={item.to} item={item} />
            ))}
          </div>
        </nav>

        <div className="border-t border-line px-3 py-3">
          <div className="flex items-center justify-between px-1 pb-2">
            <span className="text-[10px] font-medium uppercase tracking-[0.08em] text-faint">Version</span>
            <span className="font-mono text-[10px] text-faint">{formatVersion(meta.data?.version)}</span>
          </div>

          <div className="mt-1 flex items-center gap-2 rounded-md px-1 py-1">
            <span className="grid size-8 shrink-0 place-items-center rounded-full bg-accent-soft font-mono text-[10px] font-semibold text-accent">
              {initials(user)}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[11px] font-semibold text-ink">
                {user.display_name || user.email}
              </p>
              <p className="truncate text-[10px] text-faint">{titleCase(user.role)}</p>
            </div>
            {demoMode ? (
              <StatusPill tone="warn" dot={false}>
                Demo
              </StatusPill>
            ) : (
              <button
                type="button"
                onClick={() => void signOut()}
                disabled={signingOut}
                className="grid size-7 place-items-center rounded-md text-faint transition-colors hover:bg-surface-2 hover:text-ink disabled:opacity-40"
                aria-label="Sign out"
                title="Sign out"
              >
                <LogOutIcon className="size-3.5" />
              </button>
            )}
          </div>

          <div className="mt-3 flex items-center justify-between gap-3 px-0.5">
            <span className="text-[11px] text-faint">Appearance</span>
            <div className="w-[96px]">
              <ThemeControl />
            </div>
          </div>
        </div>
      </aside>

      <div className="lg:pl-60">
        <header className="sticky top-0 z-20 border-b border-line bg-canvas/90 backdrop-blur-md">
          <div className="mx-auto flex h-16 max-w-6xl items-center justify-between gap-3 px-4 sm:px-6 lg:px-8">
            <div className="flex min-w-0 items-center gap-3">
              <Link to="/" className="flex items-center gap-2 lg:hidden" aria-label="IWMMD overview">
                <Mark className="size-8 text-accent" />
                <span className="text-sm font-semibold tracking-[-0.03em] text-ink">IWMMD</span>
              </Link>
              <div className="hidden min-w-0 lg:block">
                <p className="truncate text-[13px] font-semibold tracking-[-0.01em] text-ink">{page.title}</p>
                <p className="truncate text-[10px] font-medium tracking-[0.08em] text-faint">{page.detail}</p>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <div className="lg:hidden">
                <ThemeControl compact />
              </div>
              <div className="flex h-8 items-center gap-2 rounded-md border border-line bg-surface px-2.5 text-[11px] font-medium text-muted">
                <ShieldIcon className="size-3.5 text-faint" />
                <span className="max-w-[10rem] truncate">{user.display_name || user.email}</span>
                <StatusPill tone={user.role === 'admin' ? 'ok' : 'neutral'} dot={false}>
                  {titleCase(user.role)}
                </StatusPill>
              </div>
            </div>
          </div>

          <nav className="flex gap-1 overflow-x-auto border-t border-line px-3 py-1.5 lg:hidden" aria-label="Primary mobile">
            {navItems.map((item) => (
              <NavLink key={item.to} item={item} mobile />
            ))}
          </nav>
        </header>

        <main className="mx-auto max-w-6xl px-4 py-6 sm:px-6 sm:py-8 lg:px-8">
          {demoMode ? (
            <div className="mb-5">
              <Banner tone="warn" title="Public demo">
                Mock data resets automatically at {formatDateTime(meta.data?.demo_reset_at, 'the next cycle')}.
                Changes are shared by everyone using this demo and are temporary. Checks and notifications are simulated.
              </Banner>
            </div>
          ) : null}
          <Outlet />
        </main>
      </div>
    </div>
  )
}

function RedirectToLogin() {
  useEffect(() => {
    const next = `${window.location.pathname}${window.location.search}`
    window.location.replace(`/login?next=${encodeURIComponent(next)}`)
  }, [])
  return (
    <CenteredState title="Sign in required">
      <p>Redirecting to the sign-in page…</p>
      <p className="mt-3">
        If nothing happens, open <a className="text-accent hover:underline" href="/login">the login page</a>.
      </p>
    </CenteredState>
  )
}

export function AppShell() {
  const me = useMe()
  const meta = useMeta()

  if (me.isPending || meta.isPending) {
    return (
      <CenteredState title="Starting IWMMD">
        <Spinner label="Checking your session" />
      </CenteredState>
    )
  }

  if (me.isError) {
    return (
      <CenteredState title="Cannot reach the API">
        <Banner tone="error" title="Session check failed">
          {me.error instanceof Error ? me.error.message : 'The server did not answer.'}
        </Banner>
        <button
          type="button"
          onClick={() => window.location.reload()}
          className="mt-4 inline-flex h-9 items-center rounded-md border border-line bg-surface px-3 text-xs font-semibold text-ink hover:bg-surface-2"
        >
          Try again
        </button>
      </CenteredState>
    )
  }

  if (!meta.data?.auth_configured) {
    return (
      <CenteredState title="Authentication is not configured">
        <p>
          IWMMD uses OIDC only. Set the issuer, client ID, client secret, redirect URL, and an admin source,
          then restart the process.
        </p>
        <p className="mt-3">
          The interface is intentionally unavailable until sign-in works. Configuration details are in the
          README and <span className="font-mono text-[10px] text-ink">config.example.toml</span>.
        </p>
      </CenteredState>
    )
  }

  if (!me.data?.user) {
    return <RedirectToLogin />
  }

  return <Shell user={me.data.user} />
}
