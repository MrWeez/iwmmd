import { Link } from '@tanstack/react-router'
import { useMeta } from '../lib/api'
import { AlertIcon, ChevronRightIcon } from './icons'

export function AuthNotice() {
  const meta = useMeta()
  if (meta.isPending || meta.data?.auth_configured) {
    return null
  }

  return (
    <div className="flex flex-col gap-3 rounded-[10px] border border-signal/30 bg-signal/[0.08] px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex gap-3">
        <AlertIcon className="mt-0.5 size-4 shrink-0 text-signal" />
        <div>
          <p className="text-xs font-semibold text-ink">OIDC is not configured yet</p>
          <p className="mt-0.5 text-[11px] leading-5 text-muted">
            The interface is read-only until authentication is connected. Add an issuer, client, and admin source to enable writes.
          </p>
        </div>
      </div>
      <Link
        to="/settings"
        className="inline-flex shrink-0 items-center gap-0.5 text-[11px] font-semibold text-signal hover:underline"
      >
        View configuration
        <ChevronRightIcon className="size-3" />
      </Link>
    </div>
  )
}
