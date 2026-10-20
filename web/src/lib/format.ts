import type { Domain, Rule } from './api'

export function formatVersion(version: string | undefined): string {
  if (!version || version === 'dev') {
    return 'dev'
  }
  return version.startsWith('v') ? version : `v${version}`
}

export function shortCommit(commit: string | undefined): string {
  if (!commit || commit === 'none') {
    return 'none'
  }
  return commit.slice(0, 7)
}

export function formatDate(value: string | undefined, fallback = 'Unknown'): string {
  if (!value) {
    return fallback
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return fallback
  }
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
  }).format(date)
}

export function formatDateTime(value: string | undefined, fallback = 'Never'): string {
  if (!value) {
    return fallback
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return fallback
  }
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

export function relativeTime(value: string | undefined): string {
  if (!value) {
    return 'never'
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'never'
  }
  const seconds = Math.round((date.getTime() - Date.now()) / 1000)
  const formatter = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' })
  const ranges: Array<[Intl.RelativeTimeFormatUnit, number]> = [
    ['year', 31_536_000],
    ['month', 2_592_000],
    ['week', 604_800],
    ['day', 86_400],
    ['hour', 3_600],
    ['minute', 60],
  ]
  for (const [unit, size] of ranges) {
    if (Math.abs(seconds) >= size) {
      return formatter.format(Math.round(seconds / size), unit)
    }
  }
  return 'just now'
}

export function daysLabel(days: number | undefined): string {
  if (days === undefined) {
    return 'Unknown'
  }
  if (days < 0) {
    return `${Math.abs(days)}d overdue`
  }
  if (days === 0) {
    return 'Today'
  }
  return `${days}d left`
}

export function domainTone(domain: Domain): 'ok' | 'warn' | 'error' | 'neutral' {
  if (domain.state === 'paused') return 'neutral'
  if (domain.expires_at === undefined || domain.days_left === undefined) return 'warn'
  if (domain.days_left <= 7) return 'error'
  if (domain.days_left <= 30) return 'warn'
  return 'ok'
}

export function offsetLabel(rule: Pick<Rule, 'offset_value' | 'offset_unit' | 'direction'>): string {
  if (rule.offset_value === 0) {
    return 'On expiry day'
  }
  const unit = rule.offset_value === 1 ? rule.offset_unit.slice(0, -1) : rule.offset_unit
  return `${rule.offset_value} ${unit} ${rule.direction}`
}

export function expiryLabel(domain: Domain): string {
  if (!domain.expires_at) return 'Unknown'
  const date = formatDate(domain.expires_at)
  if (domain.days_left === undefined) return date
  return `${date} · ${daysLabel(domain.days_left)}`
}

export function toDateTimeLocal(value?: string): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60_000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

export function fromDateTimeLocal(value: string): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toISOString()
}

export function titleCase(value: string): string {
  return value
    .split(/[\s_-]+/)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

export function parseRecipients(value: string): string[] {
  return value
    .split(/[\n,;]+/)
    .map((item) => item.trim())
    .filter(Boolean)
}
