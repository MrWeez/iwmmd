import { Link } from '@tanstack/react-router'
import { useMeta } from '../lib/api'
import { CheckIcon, ChevronRightIcon } from './icons'
import { Panel, StatusPill } from './ui'

type Step = {
  title: string
  detail: string
  to: string
  linkLabel: string
  done: boolean
}

export function SetupChecklist({
  domainCount = 0,
  ruleCount = 0,
  channelCount = 0,
}: {
  domainCount?: number
  ruleCount?: number
  channelCount?: number
}) {
  const meta = useMeta()
  const steps: Step[] = [
    {
      title: 'Connect OIDC',
      detail: 'Sign-in is active. Admin access comes from groups or a named email.',
      to: '/settings',
      linkLabel: 'View settings',
      done: meta.data?.auth_configured ?? false,
    },
    {
      title: 'Add a domain',
      detail: 'IWMMD checks RDAP first, then falls back to WHOIS when a registry does not answer.',
      to: '/domains',
      linkLabel: 'Open domains',
      done: domainCount > 0,
    },
    {
      title: 'Choose alert offsets',
      detail: 'Set reminders in days or calendar months, before or after expiry, with your own send time.',
      to: '/alerts',
      linkLabel: 'Open alerts',
      done: ruleCount > 0,
    },
    {
      title: 'Connect a channel',
      detail: 'Email, a generic webhook, Discord, or Telegram. Mute any destination when you need quiet.',
      to: '/channels',
      linkLabel: 'Open channels',
      done: channelCount > 0,
    },
  ]

  const complete = steps.filter((step) => step.done).length
  const nextIndex = steps.findIndex((step) => !step.done)

  return (
    <Panel
      title="Setup checklist"
      description="Four steps to a live watchlist."
      action={
        <StatusPill tone={complete === steps.length ? 'ok' : 'neutral'} dot={false}>
          {complete} of {steps.length}
        </StatusPill>
      }
    >
      <ol className="divide-y divide-line">
        {steps.map((step, index) => {
          const isNext = index === nextIndex
          return (
            <li key={step.title} aria-current={isNext ? 'step' : undefined}>
              <Link
                to={step.to}
                className="group flex gap-3 px-4 py-3.5 transition-colors hover:bg-surface-2/60 sm:px-5"
              >
                <span
                  className={`mt-0.5 grid size-6 shrink-0 place-items-center rounded-full border font-mono text-[10px] ${
                    step.done
                      ? 'border-accent bg-accent text-canvas'
                      : isNext
                        ? 'border-signal text-signal'
                        : 'border-line text-faint'
                  }`}
                >
                  {step.done ? <CheckIcon className="size-3.5" /> : String(index + 1).padStart(2, '0')}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-2">
                    <span className={`text-xs font-semibold ${step.done ? 'text-muted' : 'text-ink'}`}>
                      {step.title}
                    </span>
                    {isNext ? (
                      <StatusPill tone="warn" dot={false}>
                        Next
                      </StatusPill>
                    ) : null}
                  </span>
                  <span className="mt-0.5 block text-[11px] leading-5 text-muted">{step.detail}</span>
                  <span className="mt-1.5 inline-flex items-center gap-0.5 text-[11px] font-medium text-accent">
                    {step.linkLabel}
                    <ChevronRightIcon className="size-3 transition-transform group-hover:translate-x-0.5" />
                  </span>
                </span>
              </Link>
            </li>
          )
        })}
      </ol>
    </Panel>
  )
}
