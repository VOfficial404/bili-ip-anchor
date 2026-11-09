import { useState } from 'react'
import type { Account } from '@/lib/api'
import { avatarColor, hdslbURL, initials } from '@/lib/format'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'

type Tone = 'info' | 'danger' | 'muted' | 'success' | 'warning'

export function accountState(a: Account): [string, Tone] {
  if (a.running) return ['运行中', 'info']
  if (a.busy) return [`正在${a.busy}`, 'info']
  if (!a.logged_in) return ['登录失效', 'danger']
  if (!a.last) return ['等待首次运行', 'muted']
  return a.last.ok ? ['正常', 'success'] : ['上次失败', 'warning']
}

const TONE: Record<Tone, string> = {
  info: 'border-info/30 bg-info/10 text-info',
  danger: 'border-destructive/30 bg-destructive/10 text-destructive',
  muted: 'border-border bg-muted text-muted-foreground',
  success: 'border-success/30 bg-success/10 text-success',
  warning: 'border-warning/30 bg-warning/10 text-warning',
}

export function ToneBadge({
  tone,
  className,
  children,
}: {
  tone: Tone
  className?: string
  children: React.ReactNode
}) {
  return (
    <Badge variant='outline' className={cn(TONE[tone], className)}>
      {children}
    </Badge>
  )
}

export function StateBadge({ account }: { account: Account }) {
  const [text, tone] = accountState(account)
  return (
    <ToneBadge tone={tone}>
      {(tone === 'info') && <span className='size-1.5 animate-pulse rounded-full bg-current' />}
      {text}
    </ToneBadge>
  )
}

// AccountAvatar 显示 B 站头像，加载失败或没有时显示首字
export function AccountAvatar({ account, className }: { account: Account; className?: string }) {
  const label = account.uname || `UID ${account.mid}`
  const src = hdslbURL(account.face, '@96w_96h_1c.webp')
  const [loaded, setLoaded] = useState('')
  const show = src && loaded === src
  return (
    <span
      className={cn(
        'relative grid shrink-0 place-items-center overflow-hidden rounded-full font-semibold text-white select-none',
        AVATAR_BG[avatarColor(label)],
        className
      )}
      aria-hidden='true'
    >
      {!show && <span>{initials(label)}</span>}
      {src && (
        <img
          key={src}
          src={src}
          alt=''
          referrerPolicy='no-referrer'
          className={cn('absolute inset-0 size-full object-cover', !show && 'invisible')}
          onLoad={() => setLoaded(src)}
          onError={() => setLoaded('')}
        />
      )}
    </span>
  )
}

// 头像底色用类名而不是行内样式，CSP 不需要放开 style 属性
const AVATAR_BG: Record<string, string> = {
  '#e11d48': 'bg-rose-600',
  '#db2777': 'bg-pink-600',
  '#c026d3': 'bg-fuchsia-600',
  '#9333ea': 'bg-purple-600',
  '#4f46e5': 'bg-indigo-600',
  '#0284c7': 'bg-sky-600',
  '#0891b2': 'bg-cyan-600',
  '#0d9488': 'bg-teal-600',
  '#059669': 'bg-emerald-600',
  '#65a30d': 'bg-lime-600',
  '#ca8a04': 'bg-yellow-600',
  '#ea580c': 'bg-orange-600',
}
