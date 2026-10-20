import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark' | 'system'

const storageKey = 'iwmmd-theme'

export function storedTheme(): Theme {
  try {
    const value = localStorage.getItem(storageKey)
    if (value === 'light' || value === 'dark' || value === 'system') {
      return value
    }
  } catch {
    // Storage can be unavailable in hardened browser contexts.
  }
  return 'system'
}

export function resolvedTheme(theme: Theme): 'light' | 'dark' {
  if (theme !== 'system') {
    return theme
  }
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function applyTheme(theme: Theme) {
  const resolved = resolvedTheme(theme)
  document.documentElement.classList.toggle('dark', resolved === 'dark')
  document.documentElement.dataset.theme = resolved
}

export function initialiseTheme() {
  applyTheme(storedTheme())
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(storedTheme)

  useEffect(() => {
    applyTheme(theme)
    if (theme !== 'system') {
      return
    }
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => applyTheme('system')
    media.addEventListener('change', onChange)
    return () => media.removeEventListener('change', onChange)
  }, [theme])

  useEffect(() => {
    const onStorage = (event: StorageEvent) => {
      if (event.key === storageKey) {
        setThemeState(storedTheme())
      }
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  const setTheme = (next: Theme) => {
    setThemeState(next)
    try {
      localStorage.setItem(storageKey, next)
    } catch {
      // The in-memory theme still applies for this session.
    }
  }

  return { theme, setTheme }
}
