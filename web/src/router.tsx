import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import { RootLayout } from './components/RootLayout'
import { AlertsPage } from './pages/AlertsPage'
import { ChannelsPage } from './pages/ChannelsPage'
import { DomainsPage } from './pages/DomainsPage'
import { IncidentsPage } from './pages/IncidentsPage'
import { LoginPage } from './pages/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'
import { OverviewPage } from './pages/OverviewPage'
import { SettingsPage } from './pages/SettingsPage'

const rootRoute = createRootRoute({
  component: RootLayout,
  notFoundComponent: NotFoundPage,
})

const overviewRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/',
  component: OverviewPage,
})

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/login',
  component: LoginPage,
})

const domainsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/domains',
  component: DomainsPage,
})

const alertsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/alerts',
  component: AlertsPage,
})

const channelsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/channels',
  component: ChannelsPage,
})

const incidentsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/incidents',
  component: IncidentsPage,
})

const settingsRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/settings',
  component: SettingsPage,
})

const routeTree = rootRoute.addChildren([
  loginRoute,
  overviewRoute,
  domainsRoute,
  alertsRoute,
  channelsRoute,
  incidentsRoute,
  settingsRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: 'intent',
  scrollRestoration: true,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}
