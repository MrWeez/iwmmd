import { MonitorIcon, MoonIcon, SunIcon } from './icons'
import { useTheme, type Theme } from '../lib/theme'

const options: Array<{ value: Theme; label: string; Icon: typeof SunIcon }> = [
  { value: 'light', label: 'Light theme', Icon: SunIcon },
  { value: 'dark', label: 'Dark theme', Icon: MoonIcon },
  { value: 'system', label: 'System theme', Icon: MonitorIcon },
]

export function ThemeControl({ compact = false }: { compact?: boolean }) {
  const { theme, setTheme } = useTheme()

  if (compact) {
    const current = options.find((option) => option.value === theme) ?? options[2]
    const next = theme === 'light' ? 'dark' : theme === 'dark' ? 'system' : 'light'
    return (
      <button
        type="button"
        onClick={() => setTheme(next)}
        className="grid size-9 place-items-center rounded-md border border-line bg-surface text-muted transition-colors hover:bg-surface-2 hover:text-ink"
        aria-label={`${current.label}. Switch to ${next} theme.`}
        title={`${current.label}. Switch to ${next} theme.`}
      >
        <current.Icon className="size-4" />
      </button>
    )
  }

  return (
    <div
      className="grid grid-cols-3 gap-0.5 rounded-md border border-line bg-surface-2 p-0.5"
      role="group"
      aria-label="Theme"
    >
      {options.map(({ value, label, Icon }) => {
        const active = theme === value
        return (
          <button
            key={value}
            type="button"
            onClick={() => setTheme(value)}
            aria-label={label}
            aria-pressed={active}
            title={label}
            className={`grid h-7 place-items-center rounded-[5px] transition-colors ${
              active ? 'bg-surface text-ink shadow-[0_0_0_1px_var(--line)]' : 'text-faint hover:text-ink'
            }`}
          >
            <Icon className="size-3.5" />
          </button>
        )
      })}
    </div>
  )
}
