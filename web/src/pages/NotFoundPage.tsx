import { Link } from '@tanstack/react-router'
import { GlobeIcon } from '../components/icons'
import { EmptyState, PageHeader } from '../components/ui'

export function NotFoundPage() {
  return (
    <div className="space-y-5">
      <PageHeader title="Not found" description="That route does not exist in this build." />
      <EmptyState
        icon={GlobeIcon}
        title="Nothing lives here"
        description="Check the address, or return to the overview to see the watchlist."
        action={
          <Link
            to="/"
            className="inline-flex h-9 items-center justify-center rounded-md bg-accent px-3 text-xs font-semibold text-canvas transition-colors hover:bg-accent/90"
          >
            Back to overview
          </Link>
        }
      />
    </div>
  )
}
