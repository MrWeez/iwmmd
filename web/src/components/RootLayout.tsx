import { Outlet, useRouterState } from '@tanstack/react-router'
import { AppShell } from './AppShell'

export function RootLayout() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  if (pathname === '/login') {
    return <Outlet />
  }
  return <AppShell />
}
