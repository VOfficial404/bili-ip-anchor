import { useEffect, useMemo, useRef, useState } from 'react'
import { ArrowDown, Search as SearchIcon } from 'lucide-react'
import { apiJSON, Unauthorized, type LogEntry, type LogLevel } from '@/lib/api'
import { fmtLogTime } from '@/lib/format'
import { useStatusData } from '@/lib/status'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Page } from '@/components/layout/page'

const MAX = 1000 // 和服务端日志缓冲一样多

const LEVEL_RANK: Record<LogLevel, number> = { debug: 0, info: 1, warn: 2, error: 3 }

const LEVEL_BADGE: Record<LogLevel, string> = {
  debug: 'border-border text-muted-foreground',
  info: 'border-info/30 bg-info/10 text-info',
  warn: 'border-warning/30 bg-warning/10 text-warning',
  error: 'border-destructive/30 bg-destructive/10 text-destructive',
}

const LEVEL_NAME: Record<LogLevel, string> = { debug: 'DEBUG', info: 'INFO', warn: 'WARN', error: 'ERROR' }

// useLogs 拉一次全部，之后每 3 秒只拉序号更大的
function useLogs() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [error, setError] = useState('')
  const last = useRef(0)

  useEffect(() => {
    let stop = false
    let timer: ReturnType<typeof setTimeout>
    async function tick() {
      try {
        const r = await apiJSON<{ level: LogLevel; entries: LogEntry[] }>('GET', `/api/logs?after=${last.current}`)
        if (stop) return
        setError('')
        if (r.entries.length) {
          last.current = r.entries[r.entries.length - 1].seq
          setEntries((old) => {
            const all = old.concat(r.entries)
            return all.length > MAX ? all.slice(all.length - MAX) : all
          })
        }
      } catch (err) {
        if (stop || err instanceof Unauthorized) return
        setError((err as Error).message)
      }
      if (!stop) timer = setTimeout(tick, 3000)
    }
    tick()
    return () => {
      stop = true
      clearTimeout(timer)
    }
  }, [])

  return { entries, error }
}

export function Logs() {
  const s = useStatusData()
  const { entries, error } = useLogs()
  const [level, setLevel] = useState<'all' | LogLevel>('all')
  const [mid, setMid] = useState('all')
  const [q, setQ] = useState('')
  const box = useRef<HTMLDivElement>(null)
  const [atBottom, setAtBottom] = useState(true)

  // 账号筛选：现有的账号加上日志里出现过的
  const accounts = useMemo(() => {
    const m = new Map<string, string>()
    for (const a of s.accounts) m.set(a.mid, a.uname || `UID ${a.mid}`)
    for (const e of entries) if (e.mid && !m.has(e.mid)) m.set(e.mid, e.account || `UID ${e.mid}`)
    return [...m]
  }, [s.accounts, entries])

  const shown = useMemo(() => {
    const needle = q.trim().toLowerCase()
    return entries.filter(
      (e) =>
        (level === 'all' || LEVEL_RANK[e.level] >= LEVEL_RANK[level]) &&
        (mid === 'all' || (mid === 'none' ? !e.mid : e.mid === mid)) &&
        (!needle || e.msg.toLowerCase().includes(needle) || (e.account ?? '').toLowerCase().includes(needle))
    )
  }, [entries, level, mid, q])

  // 在底部时跟着新日志滚动
  useEffect(() => {
    const el = box.current
    if (el && atBottom) el.scrollTop = el.scrollHeight
  }, [shown, atBottom])

  const toBottom = () => {
    const el = box.current
    if (el) el.scrollTop = el.scrollHeight
    setAtBottom(true)
  }

  const current = LEVEL_NAME[s.log_level] ?? 'INFO'

  return (
    <Page
      title='日志'
      fixed
      lead={`最近 ${MAX} 条，每 3 秒更新。现在记录 ${current} 及以上的日志，可以在「设置」里调整。`}
    >
      <div className='flex flex-wrap gap-2'>
        <div className='relative min-w-0 flex-1 basis-48'>
          <SearchIcon className='pointer-events-none absolute start-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground' />
          <Input
            type='search'
            aria-label='搜索日志'
            placeholder='搜索日志'
            className='ps-8'
            value={q}
            onChange={(e) => setQ(e.target.value)}
          />
        </div>
        <Select value={level} onValueChange={(v) => setLevel(v as typeof level)}>
          <SelectTrigger aria-label='级别' className='w-[calc(50%-0.25rem)] sm:w-36'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>全部级别</SelectItem>
            <SelectItem value='debug'>DEBUG 及以上</SelectItem>
            <SelectItem value='info'>INFO 及以上</SelectItem>
            <SelectItem value='warn'>WARN 及以上</SelectItem>
            <SelectItem value='error'>只看 ERROR</SelectItem>
          </SelectContent>
        </Select>
        <Select value={mid} onValueChange={setMid}>
          <SelectTrigger aria-label='账号' className='w-[calc(50%-0.25rem)] sm:w-44'>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>全部账号</SelectItem>
            <SelectItem value='none'>和账号无关的</SelectItem>
            {accounts.map(([id, name]) => (
              <SelectItem key={id} value={id}>
                {name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {error && (
        <p role='alert' className='text-sm text-destructive'>
          日志更新失败：{error}
        </p>
      )}

      <Card className='relative min-h-0 flex-1 gap-0 overflow-hidden py-0'>
        <div
          ref={box}
          tabIndex={0}
          aria-label='日志'
          className='h-full min-h-72 overflow-auto font-mono text-xs leading-relaxed'
          onScroll={(e) => {
            const el = e.currentTarget
            setAtBottom(el.scrollHeight - el.scrollTop - el.clientHeight < 24)
          }}
        >
          {shown.length === 0 ? (
            <p className='p-4 text-muted-foreground'>{entries.length ? '没有符合条件的日志' : '还没有日志'}</p>
          ) : (
            <ol className='divide-y divide-border/60'>
              {shown.map((e) => (
                <li
                  key={e.seq}
                  className={cn(
                    'flex flex-wrap items-baseline gap-x-2 px-3 py-1 sm:flex-nowrap',
                    e.level === 'error' && 'bg-destructive/5',
                    e.level === 'warn' && 'bg-warning/5'
                  )}
                >
                  <time className='shrink-0 text-muted-foreground' dateTime={e.time}>
                    {fmtLogTime(e.time)}
                  </time>
                  <span
                    className={cn(
                      'inline-flex w-12 shrink-0 justify-center rounded border px-1 text-[10px] leading-4 font-semibold',
                      LEVEL_BADGE[e.level]
                    )}
                  >
                    {LEVEL_NAME[e.level]}
                  </span>
                  {e.account && (
                    <span className='max-w-40 shrink-0 truncate text-info' title={`UID ${e.mid}`}>
                      [{e.account}]
                    </span>
                  )}
                  <span
                    className={cn(
                      'min-w-0 basis-full break-words whitespace-pre-wrap sm:basis-auto',
                      e.level === 'error' && 'text-destructive',
                      e.level === 'debug' && 'text-muted-foreground'
                    )}
                  >
                    {e.msg}
                  </span>
                </li>
              ))}
            </ol>
          )}
        </div>
        {!atBottom && (
          <Button size='sm' variant='secondary' className='absolute end-3 bottom-3 shadow-md' onClick={toBottom}>
            <ArrowDown />
            最新
          </Button>
        )}
      </Card>
    </Page>
  )
}
