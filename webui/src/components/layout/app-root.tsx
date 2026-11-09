import { useEffect } from 'react'
import { Outlet, useLocation } from '@tanstack/react-router'
import { LoaderCircle } from 'lucide-react'
import { setAuth, useAuth, Unauthorized } from '@/lib/api'
import { useStatus } from '@/lib/status'
import { PAGES, type PagePath } from '@/router'
import { ConfirmProvider } from '@/context/confirm-provider'
import { QRProvider } from '@/context/qr-provider'
import { SearchProvider } from '@/context/search-provider'
import { Toaster } from '@/components/ui/sonner'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from '@/components/layout/app-sidebar'
import { SkipToMain } from '@/components/skip-to-main'
import { Login } from '@/features/login'

// AppRoot 先问服务端有没有有效会话：有就进面板，没有就显示密码页
export function AppRoot() {
  const auth = useAuth()
  const status = useStatus(auth !== 'out')

  useEffect(() => {
    if (status.isSuccess) setAuth('in')
    else if (status.error instanceof Unauthorized) setAuth('out')
  }, [status.isSuccess, status.error, status.dataUpdatedAt])

  const path = useLocation({ select: (l) => l.pathname })
  useEffect(() => {
    const title = PAGES[path as PagePath]
    document.title = title ? `${title} · Bili IP Anchor` : 'Bili IP Anchor'
  }, [path])

  let body: React.ReactNode
  if (auth === 'out') body = <Login />
  else if (!status.data)
    body = status.error ? (
      <div className='flex h-svh flex-col items-center justify-center gap-2 p-6 text-center'>
        <p className='font-medium'>连不上面板服务</p>
        <p className='text-sm text-muted-foreground'>
          {status.error.message}，正在重试
        </p>
      </div>
    ) : (
      <div className='flex h-svh items-center justify-center text-muted-foreground'>
        <LoaderCircle className='size-5 animate-spin' aria-label='正在连接' />
      </div>
    )
  else
    body = (
      <ConfirmProvider>
        <QRProvider>
          <SearchProvider>
            <SidebarProvider defaultOpen={!document.cookie.includes('sidebar_state=false')}>
              <SkipToMain />
              <AppSidebar />
              <SidebarInset className='@container/content min-w-0'>
                <Outlet />
              </SidebarInset>
            </SidebarProvider>
          </SearchProvider>
        </QRProvider>
      </ConfirmProvider>
    )

  return (
    <>
      {body}
      <Toaster duration={4000} position='top-center' />
    </>
  )
}
