import { Link, useRouterState } from '@tanstack/react-router'
import { useState } from 'react'
import { useMeta, useMe } from '../lib/api'
import { formatVersion } from '../lib/format'
import { Mark, ShieldIcon } from '../components/icons'
import { Banner, PrimaryButton, Spinner, StatusPill } from '../components/ui'
import { ThemeControl } from '../components/ThemeControl'

const errors: Record<string, string> = {
  state: 'The sign-in request expired or did not match. Start again.',
  provider: 'The identity provider returned an error.',
  exchange: 'The provider response could not be verified. Check the server logs.',
  storage: 'Your account could not be stored. Try again or contact an administrator.',
  session: 'A session could not be created. Try again.',
  denied: 'Your account is not authorized for this instance.',
}

function LoginFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="grid min-h-dvh bg-canvas text-ink lg:grid-cols-[minmax(0,1fr)_28rem]">
      <main className="flex items-center justify-center px-5 py-10 sm:px-10">
        <div className="w-full max-w-md">
          <div className="mb-8 flex items-center gap-3">
            <Mark className="size-10 text-accent" />
            <div>
              <p className="text-lg font-semibold tracking-[-0.04em]">IWMMD</p>
              <p className="text-[10px] font-medium tracking-[0.14em] text-faint">DOMAIN WATCH</p>
            </div>
          </div>
          {children}
        </div>
      </main>
      <aside className="hidden border-l border-line bg-surface p-10 lg:flex lg:flex-col lg:justify-between">
        <div>
          <StatusPill tone="ok">Self-hosted</StatusPill>
          <h2 className="mt-5 text-2xl font-semibold leading-tight tracking-[-0.035em]">
            Know before a domain lapses.
          </h2>
          <p className="mt-3 text-sm leading-6 text-muted">
            One watchlist for the team. RDAP first, WHOIS fallback, and alerts that respect calendar months,
            mute windows, and interrupted infrastructure.
          </p>
        </div>
        <div className="space-y-4 border-t border-line pt-5 text-[11px] leading-5 text-muted">
          <p>Single sign-on with OIDC. No local passwords, no browser tokens.</p>
          <div className="flex items-center justify-between">
            <span className="inline-flex items-center gap-1.5">
              <ShieldIcon className="size-3.5 text-faint" />
              Server-side sessions
            </span>
            <ThemeControl compact />
          </div>
        </div>
      </aside>
    </div>
  )
}

export function LoginPage() {
  const meta = useMeta()
  const me = useMe()
  const [submitting, setSubmitting] = useState(false)
  const search = useRouterState({ select: (state) => state.location.search })
  const params = new URLSearchParams(search)
  const next = params.get('next') || '/'
  const error = params.get('error') || ''
  const configured = meta.data?.auth_configured ?? false

  const signIn = () => {
    setSubmitting(true)
    window.location.assign(`/api/v1/auth/login?next=${encodeURIComponent(next)}`)
  }

  if (me.isPending || meta.isPending) {
    return (
      <LoginFrame>
        <Spinner label="Checking this instance" />
      </LoginFrame>
    )
  }

  if (me.data?.user) {
    return (
      <LoginFrame>
        <h1 className="text-2xl font-semibold tracking-[-0.03em]">You are signed in</h1>
        <p className="mt-2 text-sm leading-6 text-muted">
          Continue as {me.data.user.display_name || me.data.user.email}.
        </p>
        <div className="mt-6">
          <Link
            to="/"
            className="inline-flex h-10 items-center justify-center rounded-md bg-accent px-4 text-xs font-semibold text-canvas transition-colors hover:bg-accent/90"
          >
            Open dashboard
          </Link>
        </div>
      </LoginFrame>
    )
  }

  return (
    <LoginFrame>
      <h1 className="text-2xl font-semibold tracking-[-0.03em]">Sign in</h1>
      <p className="mt-2 text-sm leading-6 text-muted">
        {configured
          ? `Use your organization account${meta.data?.provider ? ` through ${meta.data.provider}` : ''}.`
          : 'Authentication is not configured on this instance yet.'}
      </p>

      {error ? (
        <div className="mt-5">
          <Banner tone="error" title="Sign-in failed">
            {errors[error] ?? 'The sign-in attempt could not be completed.'}
          </Banner>
        </div>
      ) : null}

      {!configured ? (
        <div className="mt-6 space-y-4">
          <Banner tone="warn" title="OIDC is not configured">
            Set an issuer, client ID, client secret, redirect URL, and at least one admin source. Restart the
            process after changing the environment or TOML file.
          </Banner>
          <PanelCode>
            IWMMD_OIDC_ISSUER=https://id.example.com{'\n'}
            IWMMD_OIDC_CLIENT_ID=iwmmd{'\n'}
            IWMMD_OIDC_CLIENT_SECRET=...{'\n'}
            IWMMD_OIDC_REDIRECT_URL=https://iwmmd.example.com/api/v1/auth/callback{'\n'}
            IWMMD_OIDC_ADMIN_GROUPS=iwmmd-admins
          </PanelCode>
          <p className="text-[11px] leading-5 text-muted">
            The full list of environment variables and provider presets is in the README and{' '}
            <span className="font-mono text-[10px] text-ink">config.example.toml</span>.
          </p>
        </div>
      ) : (
        <div className="mt-6">
          <PrimaryButton onClick={signIn} disabled={submitting} title="Sign in with OIDC">
            <ShieldIcon className="size-3.5" />
            {submitting ? 'Redirecting…' : 'Continue with single sign-on'}
          </PrimaryButton>
          <p className="mt-4 text-[10px] leading-5 text-faint">
            You will return here after the provider verifies your account.
          </p>
        </div>
      )}

      <div className="mt-8 flex items-center justify-between border-t border-line pt-4 text-[10px] text-faint">
        <span>Version {formatVersion(meta.data?.version)}</span>
        <Link to="/" className="hover:text-ink">
          Dashboard
        </Link>
      </div>
    </LoginFrame>
  )
}

function PanelCode({ children }: { children: React.ReactNode }) {
  return (
    <pre className="overflow-x-auto rounded-md border border-line bg-surface px-3 py-3 font-mono text-[10px] leading-5 text-muted">
      {children}
    </pre>
  )
}
