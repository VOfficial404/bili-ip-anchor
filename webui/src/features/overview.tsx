import { Link } from '@tanstack/react-router'
import { CalendarClock, CircleAlert, MapPin, Users } from 'lucide-react'
import { fmtRelative, fmtTime, whereLabel, zonePlace, zoneRegion } from '@/lib/format'
import { useStatusData } from '@/lib/status'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { AccountAvatar, StateBadge } from '@/components/account-bits'
import { EmptyAccounts } from '@/components/empty-accounts'
import { AccountActions, Page } from '@/components/layout/page'

export function Overview() {
  const s = useStatusData()
  const list = s.accounts

  let bad = 0
  let authed = 0
  let matched = 0
  let next: string | null = null
  for (const a of list) {
    if (!a.logged_in || (a.last && !a.last.ok)) bad++
    if (a.app_auth) {
      authed++
      if (a.home?.matched_since) matched++
    }
    if (a.next && (!next || a.next < next)) next = a.next
  }

  // 按上次运行时间倒序，没运行过的放最后
  const runs = [...list].sort((a, b) => (b.last?.at ?? '').localeCompare(a.last?.at ?? ''))

  return (
    <Page title='概览' actions={<AccountActions />}>
      <Card aria-live='polite' className='gap-3'>
        <CardHeader>
          <CardDescription>B 站把{whereLabel(s)}识别为</CardDescription>
          <CardTitle
            className={cn(
              'text-3xl font-bold tracking-tight sm:text-4xl',
              !s.zone && 'text-muted-foreground'
            )}
          >
            {s.zone ? zonePlace(s.zone) : s.zone_error ? '暂时查不到' : '查询中'}
          </CardTitle>
        </CardHeader>
        <CardContent className='space-y-3'>
          <div className='flex flex-wrap items-center gap-2'>
            {s.zone?.isp && <Badge variant='secondary'>{s.zone.isp}</Badge>}
            {s.zone?.addr && (
              <Badge variant='outline' className='font-mono'>
                {s.zone.addr}
              </Badge>
            )}
            {s.zone && s.zone_checked_at && (
              <span className='text-xs text-muted-foreground'>查询于 {fmtTime(s.zone_checked_at)}</span>
            )}
            {!s.zone && s.zone_error && (
              <span className='text-sm text-destructive'>查询失败：{s.zone_error}</span>
            )}
          </div>
          <p className='text-sm text-muted-foreground'>
            这就是目标属地。主页的 IP 属地按省份（国外按国家）显示；在「账号」页授权查询后，可以看到每个账号主页现在显示的属地。
          </p>
        </CardContent>
      </Card>

      <div className='grid grid-cols-2 gap-3 sm:gap-4 lg:grid-cols-4'>
        <Stat title='账号' icon={Users} value={list.length} sub='各自排程、同时进行' />
        <Stat
          title='主页已是目标属地'
          icon={MapPin}
          value={authed ? `${matched} / ${authed}` : '—'}
          valueClass={authed ? 'text-success' : undefined}
          sub={authed ? `${authed} 个账号已授权查询` : '授权查询后显示'}
        />
        <Stat
          title='需要处理'
          icon={CircleAlert}
          value={bad}
          valueClass={bad > 0 ? 'text-destructive' : undefined}
          sub='登录失效或上次失败'
        />
        <Stat
          title='最近一次运行'
          icon={CalendarClock}
          value={next ? fmtTime(next) : '—'}
          sub={next ? fmtRelative(next) : '没有排程'}
        />
      </div>

      {list.length === 0 ? (
        <EmptyAccounts />
      ) : (
        <Card className='gap-4'>
          <CardHeader>
            <CardTitle>最近运行</CardTitle>
            <CardDescription>
              每个账号上一轮的结果。目标属地是 {s.zone ? zoneRegion(s.zone) : '查询中'}。
            </CardDescription>
          </CardHeader>
          <CardContent className='px-0 sm:px-6'>
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='ps-6 sm:ps-2'>账号</TableHead>
                  <TableHead>状态</TableHead>
                  <TableHead className='max-sm:hidden'>上次运行</TableHead>
                  <TableHead className='max-md:hidden'>主页属地</TableHead>
                  <TableHead className='pe-6 sm:pe-2'>下次</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {runs.map((a) => (
                  <TableRow key={a.mid}>
                    <TableCell className='ps-6 sm:ps-2'>
                      <Link to='/accounts' className='flex min-w-0 items-center gap-2 hover:underline'>
                        <AccountAvatar account={a} className='size-7 text-xs' />
                        <span className='max-w-[9rem] truncate font-medium sm:max-w-[14rem]'>
                          {a.uname || `UID ${a.mid}`}
                        </span>
                      </Link>
                    </TableCell>
                    <TableCell>
                      <StateBadge account={a} />
                    </TableCell>
                    <TableCell className='max-sm:hidden'>
                      {a.last ? (
                        <span className='flex flex-col'>
                          <span>
                            {fmtTime(a.last.at)}{' '}
                            <span className={a.last.ok ? 'text-success' : 'text-destructive'}>
                              {a.last.ok ? '成功' : '失败'}
                            </span>
                          </span>
                          {!a.last.ok && a.last.error && (
                            <span
                              className='max-w-[18rem] truncate text-xs text-muted-foreground'
                              title={a.last.error}
                            >
                              {a.last.error}
                            </span>
                          )}
                        </span>
                      ) : (
                        <span className='text-muted-foreground'>还没有运行过</span>
                      )}
                    </TableCell>
                    <TableCell className='max-md:hidden'>
                      {!a.app_auth ? (
                        <span className='text-muted-foreground'>未授权查询</span>
                      ) : a.home?.text ? (
                        <span className={a.home.matched_since ? 'text-success' : undefined}>
                          {a.home.text}
                        </span>
                      ) : (
                        <span className='text-muted-foreground'>查询中</span>
                      )}
                    </TableCell>
                    <TableCell className='pe-6 sm:pe-2'>
                      {a.running ? '正在运行' : a.next ? fmtTime(a.next) : '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardContent>
        </Card>
      )}
    </Page>
  )
}

function Stat({
  title,
  icon: Icon,
  value,
  valueClass,
  sub,
}: {
  title: string
  icon: React.ElementType
  value: React.ReactNode
  valueClass?: string
  sub: string
}) {
  return (
    <Card className='gap-2 py-4 sm:py-6'>
      <CardHeader className='flex flex-row items-center justify-between gap-2 space-y-0 px-4 sm:px-6'>
        <CardTitle className='text-sm leading-snug font-medium'>{title}</CardTitle>
        <Icon className='size-4 shrink-0 text-muted-foreground' />
      </CardHeader>
      <CardContent className='px-4 sm:px-6'>
        <div className={cn('text-xl font-bold sm:text-2xl', valueClass)}>{value}</div>
        <p className='text-xs text-muted-foreground'>{sub}</p>
      </CardContent>
    </Card>
  )
}
