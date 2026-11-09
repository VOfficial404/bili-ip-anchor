import { useState } from 'react'
import { Cookie, Globe, LogOut, MapPin, Play, RefreshCw, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { api, type Account, type Status } from '@/lib/api'
import { fmtDate, fmtRelative, fmtTime, zoneRegion } from '@/lib/format'
import { report, useRefresh, useStatusData } from '@/lib/status'
import { cn } from '@/lib/utils'
import { useConfirm } from '@/context/confirm-provider'
import { useQR } from '@/context/qr-provider'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { AccountAvatar, StateBadge, ToneBadge } from '@/components/account-bits'
import { EmptyAccounts } from '@/components/empty-accounts'
import { FavoritesRow } from '@/components/favorites'
import { AccountActions, Page } from '@/components/layout/page'

export function Accounts() {
  const s = useStatusData()
  return (
    <Page title='账号' actions={<AccountActions />}>
      {s.accounts.length === 0 ? (
        <EmptyAccounts />
      ) : (
        <ul className='grid gap-4'>
          {s.accounts.map((a) => (
            <AccountCard key={a.mid} a={a} s={s} />
          ))}
        </ul>
      )}
    </Page>
  )
}

function AccountCard({ a, s }: { a: Account; s: Status }) {
  const [pending, setPending] = useState(false)
  const refresh = useRefresh()
  const confirm = useConfirm()
  const openQR = useQR()
  const label = a.uname || `UID ${a.mid}`
  const busy = pending || a.running || Boolean(a.busy)

  async function act(method: string, action: string, done?: string) {
    setPending(true)
    try {
      await api(method, `/api/accounts/${a.mid}${action ? '/' + action : ''}`)
      if (done) toast.success(done)
    } catch (err) {
      report(err)
    }
    setPending(false)
    refresh()
  }

  const z = s.zone
  const target = z ? zoneRegion(z) : ''

  // 主页属地
  const h = a.home
  let homeValue: React.ReactNode
  let homePending = false
  let homeSub = ''
  if (!a.app_auth) {
    homeValue = '未授权查询'
    homePending = true
    homeSub = 'B 站只在 App 接口里提供主页属地，需要额外授权一次才能显示。'
  } else if (!h || (!h.text && !h.error)) {
    homeValue = '查询中'
    homePending = true
  } else if (h.error && !h.text) {
    homeValue = '查不到'
    homePending = true
    homeSub = h.error
  } else {
    let badge: React.ReactNode = null
    if (h.matched_since) {
      badge = <ToneBadge tone='success'>已是目标属地</ToneBadge>
      homeSub = `${fmtTime(h.matched_since)} 起一致`
    } else if (target) {
      badge = <ToneBadge tone='warning'>还不是{target}</ToneBadge>
      const next = a.next ? `下一轮（${fmtTime(a.next)}）` : '下一轮'
      homeSub = `预计${next}之后几小时内变为${target}，B 站没有公开具体规则`
    }
    homeValue = (
      <>
        {h.text} {badge}
      </>
    )
    if (h.error) homeSub = `上次查询失败：${h.error}`
    else if (h.checked_at) homeSub += `${homeSub ? '。' : ''}查询于 ${fmtTime(h.checked_at)}`
  }

  // 运行
  const runLines: string[] = []
  if (a.next && !a.running) runLines.push(fmtRelative(a.next))
  if (a.last) runLines.push(`上次 ${fmtTime(a.last.at)} ${a.last.ok ? '成功' : '失败'}`)
  if (a.last_ok) runLines.push(`上次从目标 IP 发出请求：${fmtTime(a.last_ok)}`)

  // Cookie
  let cookieValue: string
  let cookieTone = ''
  let cookieSub: string
  if (a.cookie.expires) {
    const days = Math.ceil((new Date(a.cookie.expires).getTime() - Date.now()) / 86400000)
    cookieValue = days > 0 ? `还有 ${days} 天` : '已过期'
    if (days <= 0) cookieTone = 'text-destructive'
    else if (days < 14) cookieTone = 'text-warning'
    cookieSub = `到 ${fmtDate(a.cookie.expires)}${a.cookie.can_refresh ? '，B 站要求时会自动刷新' : ''}`
  } else {
    cookieValue = '有效期未知'
    cookieTone = 'text-muted-foreground'
    cookieSub = a.cookie.can_refresh
      ? '下次运行时会查询'
      : '旧版本扫码登录的账号，重新扫码后可以看到有效期并刷新'
  }

  const error = !a.logged_in
    ? '这个账号的会话已失效，点「重新扫码」用同一个账号登录即可恢复。'
    : a.last && !a.last.ok
      ? a.last.error || ''
      : ''

  return (
    <li>
      <Card className='gap-0 overflow-hidden py-0'>
        <div className='flex flex-wrap items-center gap-3 p-4 sm:px-5'>
          <AccountAvatar account={a} className='size-11 text-base' />
          <div className='min-w-0 flex-1 basis-40'>
            <div className='flex min-w-0 items-center gap-2'>
              <span className='truncate font-semibold' title={label}>
                {label}
              </span>
              <StateBadge account={a} />
            </div>
            <div className='mt-1 flex flex-wrap gap-1.5'>
              <Badge variant='outline' className='font-mono text-muted-foreground'>
                UID {a.mid}
              </Badge>
              {a.stats && (
                <>
                  <Badge
                    variant='outline'
                    className={cn(
                      'font-semibold',
                      a.stats.level === 5 && 'text-amber-500',
                      a.stats.level >= 6 && 'text-red-500'
                    )}
                  >
                    Lv{a.stats.level}
                  </Badge>
                  {a.stats.vip && (
                    <Badge className='border-transparent bg-brand/15 text-brand'>{a.stats.vip}</Badge>
                  )}
                  <Badge variant='outline' className='text-muted-foreground'>
                    硬币 {a.stats.coins}
                  </Badge>
                  {a.stats.bcoins > 0 && (
                    <Badge variant='outline' className='text-muted-foreground'>
                      B币 {a.stats.bcoins}
                    </Badge>
                  )}
                </>
              )}
            </div>
          </div>
          <div className='flex items-center gap-2 max-sm:w-full'>
            {a.logged_in ? (
              <Button className='max-sm:flex-1' disabled={busy} onClick={() => act('POST', 'run')}>
                <Play />
                {a.running ? '运行中' : '立即运行'}
              </Button>
            ) : (
              <Button className='max-sm:flex-1' onClick={() => openQR()}>
                重新扫码
              </Button>
            )}
            <IconButton
              label='刷新资料'
              hint='刷新资料：只查询硬币、Cookie 和主页属地，不发心跳'
              disabled={busy || !a.logged_in}
              onClick={() => act('POST', 'check', '资料已刷新')}
            >
              <RefreshCw className={cn(a.busy === '刷新资料' && 'animate-spin')} />
            </IconButton>
            <IconButton
              label='退出'
              hint='退出这个账号'
              danger
              disabled={pending}
              onClick={() =>
                confirm({
                  title: `退出「${label}」？`,
                  desc: '这份会话会在 B 站注销并删除，之后不再为这个账号运行。',
                  confirmText: '退出',
                  destructive: true,
                  action: () => act('DELETE', '', `已退出 ${label}`),
                })
              }
            >
              <LogOut />
            </IconButton>
          </div>
        </div>

        <div className='grid gap-px border-t bg-border sm:grid-cols-2 xl:grid-cols-4'>
          <Tile icon={MapPin} title='主页属地' value={homeValue} pending={homePending} sub={homeSub}>
            {a.app_auth ? (
              <Button
                variant='link'
                size='sm'
                className='h-auto px-0 text-muted-foreground'
                disabled={busy}
                onClick={() =>
                  confirm({
                    title: `取消「${label}」的主页属地授权？`,
                    desc: '会在 B 站撤销这份 App 授权，之后不再显示主页属地。不影响定时任务。',
                    confirmText: '取消授权',
                    action: () => act('DELETE', 'appauth', '已取消授权'),
                  })
                }
              >
                取消授权
              </Button>
            ) : (
              <Button
                variant='outline'
                size='sm'
                disabled={busy || !a.logged_in}
                title='用 B 站 App 再扫一次码，授权本程序查询这个账号主页显示的 IP 属地'
                onClick={() => openQR({ mid: a.mid, label })}
              >
                <ShieldCheck />
                授权查询
              </Button>
            )}
          </Tile>
          <Tile
            icon={Globe}
            title='目标属地'
            value={z ? zoneRegion(z) : '查询中'}
            pending={!z}
            sub={z ? [z.addr, z.isp].filter(Boolean).join(' · ') : ''}
          />
          <Tile
            icon={Play}
            title='运行'
            value={a.running ? '正在运行' : a.next ? `下次 ${fmtTime(a.next)}` : '—'}
            sub={runLines.join(' · ') || '还没有运行过'}
          />
          <Tile icon={Cookie} title='Cookie' value={cookieValue} valueClass={cookieTone} sub={cookieSub}>
            <Button
              variant='outline'
              size='sm'
              disabled={busy || !a.logged_in || !a.cookie.can_refresh}
              title={
                a.cookie.can_refresh
                  ? '现在就换一份新的 Cookie'
                  : '这个账号是旧版本扫码登录的，重新扫码一次之后才能刷新'
              }
              onClick={() => act('POST', 'cookie/refresh', 'Cookie 已刷新')}
            >
              <RefreshCw className={cn(a.busy === '刷新 Cookie' && 'animate-spin')} />
              {a.busy === '刷新 Cookie' ? '正在刷新' : '刷新 Cookie'}
            </Button>
          </Tile>
        </div>

        <FavoritesRow a={a} busy={busy} />

        {error && (
          <p role='alert' className='border-t bg-destructive/5 px-4 py-3 text-sm break-words text-destructive sm:px-5'>
            {error}
          </p>
        )}
      </Card>
    </li>
  )
}

function Tile({
  icon: Icon,
  title,
  value,
  valueClass,
  pending,
  sub,
  children,
}: {
  icon: React.ElementType
  title: string
  value: React.ReactNode
  valueClass?: string
  pending?: boolean
  sub?: string
  children?: React.ReactNode
}) {
  return (
    <div className='flex min-w-0 flex-col gap-1 bg-card px-4 py-3 sm:px-5'>
      <p className='flex items-center gap-1.5 text-xs font-medium text-muted-foreground'>
        <Icon className='size-3.5' />
        {title}
      </p>
      <p
        className={cn(
          'flex flex-wrap items-center gap-2 text-base font-semibold',
          pending && 'font-medium text-muted-foreground',
          valueClass
        )}
      >
        {value}
      </p>
      {sub && <p className='text-xs break-words text-muted-foreground'>{sub}</p>}
      {children && <div className='mt-auto pt-1'>{children}</div>}
    </div>
  )
}

function IconButton({
  label,
  hint,
  danger,
  children,
  ...props
}: React.ComponentProps<typeof Button> & { label: string; hint: string; danger?: boolean }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant='outline'
          size='icon'
          aria-label={label}
          className={cn(danger && 'text-destructive hover:text-destructive')}
          {...props}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{hint}</TooltipContent>
    </Tooltip>
  )
}
