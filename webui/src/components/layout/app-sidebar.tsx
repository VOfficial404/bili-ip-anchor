import { Link, useLocation } from '@tanstack/react-router'
import { Anchor, Globe, Laptop, Lock, Server } from 'lucide-react'
import { useLock, useStatus } from '@/lib/status'
import { zonePlace, zoneRegion } from '@/lib/format'
import { cn } from '@/lib/utils'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
  useSidebar,
} from '@/components/ui/sidebar'
import { navItems } from './nav'

export function AppSidebar() {
  const { data: s, isError } = useStatus()
  const { setOpenMobile } = useSidebar()
  const path = useLocation({ select: (l) => l.pathname })
  const lock = useLock()

  const accounts = s?.accounts ?? []
  const notLoggedIn = accounts.filter((a) => !a.logged_in).length
  const local = s?.mode === 'local'

  let egress = s?.zone
    ? `${s.zone.addr} · ${zoneRegion(s.zone)}`
    : s?.zone_error
      ? '出口 IP 查询失败'
      : '出口 IP 查询中'
  if (s?.settings.proxy) egress = `经代理 ${egress}`

  return (
    <Sidebar collapsible='icon' variant='inset'>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton size='lg' asChild>
              <Link to='/' onClick={() => setOpenMobile(false)}>
                <div className='flex aspect-square size-8 items-center justify-center rounded-lg bg-brand text-white'>
                  <Anchor className='size-4' />
                </div>
                <div className='grid flex-1 text-start text-sm leading-tight'>
                  <span className='truncate font-semibold'>Bili IP Anchor</span>
                  <span className='truncate text-xs text-muted-foreground'>{s?.version}</span>
                </div>
              </Link>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>

      <SidebarContent>
        <SidebarGroup>
          <SidebarGroupLabel>面板</SidebarGroupLabel>
          <SidebarMenu>
            {navItems.map((item) => (
              <SidebarMenuItem key={item.url}>
                <SidebarMenuButton asChild isActive={path === item.url} tooltip={item.title}>
                  <Link to={item.url} onClick={() => setOpenMobile(false)}>
                    <item.icon />
                    <span>{item.title}</span>
                  </Link>
                </SidebarMenuButton>
                {item.url === '/accounts' && accounts.length > 0 && (
                  <SidebarMenuBadge
                    className={cn(notLoggedIn > 0 && 'bg-destructive/15 text-destructive')}
                    title={notLoggedIn ? `${notLoggedIn} 个账号登录失效` : `${accounts.length} 个账号`}
                  >
                    {notLoggedIn || accounts.length}
                  </SidebarMenuBadge>
                )}
              </SidebarMenuItem>
            ))}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>

      <SidebarFooter>
        <div className='space-y-1.5 px-2 pb-1 text-xs text-muted-foreground group-data-[collapsible=icon]:hidden'>
          <p className='flex items-center gap-2 font-medium text-sidebar-foreground'>
            <span
              className={cn(
                'size-2 shrink-0 rounded-full',
                isError ? 'bg-destructive' : s ? 'bg-success' : 'bg-muted-foreground'
              )}
            />
            <span className='truncate'>
              {isError
                ? '连接中断，正在重试'
                : s
                  ? `${local ? '本机运行' : '已连接'} · ${s.version}`
                  : '正在连接'}
            </span>
          </p>
          <p className='flex items-center gap-2' title='面板地址'>
            {local ? <Laptop className='size-3.5 shrink-0' /> : <Server className='size-3.5 shrink-0' />}
            <span className='truncate font-mono'>{local ? '在这台设备上运行' : location.host}</span>
          </p>
          <p
            className='flex items-center gap-2'
            title={
              s?.zone
                ? `B 站看到的出口 IP：${s.zone.addr}（${zonePlace(s.zone)} ${s.zone.isp || ''}）`
                : 'B 站看到的出口 IP'
            }
          >
            <Globe className='size-3.5 shrink-0' />
            <span className='truncate'>{egress}</span>
          </p>
        </div>
        {/* 本机模式（手机 App）的面板密码是随机的，锁上就进不来了 */}
        {s && !local && (
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton tooltip='锁定面板' onClick={lock}>
                <Lock />
                <span>锁定面板</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        )}
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
