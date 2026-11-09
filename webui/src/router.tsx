import {
  createHashHistory,
  createRootRoute,
  createRoute,
  createRouter,
} from '@tanstack/react-router'
import { AppRoot } from '@/components/layout/app-root'
import { NotFound } from '@/features/not-found'
import { Overview } from '@/features/overview'
import { Accounts } from '@/features/accounts'
import { SettingsPage } from '@/features/settings'
import { Logs } from '@/features/logs'

// 面板由 Go 的静态文件服务提供，没有 SPA 回退，所以用 hash 路由：/#/accounts
const rootRoute = createRootRoute({
  component: AppRoot,
  notFoundComponent: NotFound,
})

const getParentRoute = () => rootRoute

const routeTree = rootRoute.addChildren([
  createRoute({ getParentRoute, path: '/', component: Overview }),
  createRoute({ getParentRoute, path: '/accounts', component: Accounts }),
  createRoute({ getParentRoute, path: '/settings', component: SettingsPage }),
  createRoute({ getParentRoute, path: '/logs', component: Logs }),
])

export const router = createRouter({
  routeTree,
  history: createHashHistory(),
  defaultPreload: false,
})

declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

export const PAGES = {
  '/': '概览',
  '/accounts': '账号',
  '/settings': '设置',
  '/logs': '日志',
} as const

export type PagePath = keyof typeof PAGES
