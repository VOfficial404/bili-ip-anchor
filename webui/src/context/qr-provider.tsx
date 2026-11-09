import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { LoaderCircle } from 'lucide-react'
import { toast } from 'sonner'
import { api, apiJSON, Unauthorized } from '@/lib/api'
import { useRefresh } from '@/lib/status'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'

// 扫码：添加账号（target 为空），或授权查询某个账号的主页属地
type Target = { mid: string; label: string } | null

const QRContext = createContext<((t?: Target) => void) | null>(null)

const QR_TEXT: Record<string, string> = {
  waiting: '用 B 站 App 扫一扫',
  scanned: '已扫码，请在手机上确认',
  expired: '二维码已过期',
}

type QRState = { text: string; kind?: 'error' | 'scanned'; dim: boolean; renew: boolean }

export function QRProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  const [target, setTarget] = useState<Target>(null)
  const [img, setImg] = useState('')
  const [st, setSt] = useState<QRState>({ text: '正在获取二维码', dim: true, renew: false })
  const poll = useRef<ReturnType<typeof setInterval> | null>(null)
  const gen = useRef(0) // 每次开始新的二维码加一，丢掉旧请求的结果
  const refresh = useRefresh()
  const navigate = useNavigate()

  const stop = useCallback(() => {
    if (poll.current) clearInterval(poll.current)
    poll.current = null
  }, [])

  const failed = useCallback(
    (text: string, kind?: 'error') => {
      stop()
      setSt({ text, kind, dim: true, renew: true })
    },
    [stop]
  )

  const start = useCallback(
    async (t: Target) => {
      stop()
      const my = ++gen.current
      const path = t ? `/api/accounts/${t.mid}/appauth` : '/api/qr'
      setSt({ text: '正在获取二维码', dim: true, renew: false })
      try {
        const blob = await (await api('POST', path)).blob()
        if (my !== gen.current) return
        setImg((old) => {
          if (old) URL.revokeObjectURL(old)
          return URL.createObjectURL(blob)
        })
        setSt({ text: QR_TEXT.waiting, dim: false, renew: false })
        poll.current = setInterval(async () => {
          try {
            const { state, mid } = await apiJSON<{ state: string; mid?: string }>('GET', path)
            if (my !== gen.current) return
            if (state === 'success') {
              stop()
              setOpen(false)
              if (t) toast.success(`已授权，正在查询「${t.label}」的主页属地`)
              else {
                toast.success(`已添加账号 UID ${mid}，正在运行第一轮`)
                navigate({ to: '/accounts' })
              }
              refresh()
              setTimeout(refresh, 3000)
            } else if (state === 'expired' || state === 'none') {
              failed(QR_TEXT.expired)
            } else if (QR_TEXT[state]) {
              setSt({ text: QR_TEXT[state], kind: state === 'scanned' ? 'scanned' : undefined, dim: false, renew: false })
            }
          } catch (err) {
            if (err instanceof Unauthorized) return setOpen(false)
            failed((err as Error).message, 'error')
          }
        }, 2000)
      } catch (err) {
        if (my !== gen.current) return
        if (err instanceof Unauthorized) return setOpen(false)
        failed((err as Error).message, 'error')
      }
    },
    [stop, failed, refresh, navigate]
  )

  const openQR = useCallback(
    (t: Target = null) => {
      setTarget(t)
      setOpen(true)
      start(t)
    },
    [start]
  )

  // 关闭时停止轮询
  useEffect(() => {
    if (!open) {
      gen.current++
      stop()
    }
  }, [open, stop])

  useEffect(() => () => stop(), [stop])

  return (
    <QRContext value={openQR}>
      {children}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className='sm:max-w-sm'>
          <DialogHeader>
            <DialogTitle>{target ? '授权查询主页属地' : '添加 B 站账号'}</DialogTitle>
            <DialogDescription>
              {target
                ? `用「${target.label}」的 B 站 App 扫码并确认。这份授权只用来查询这个账号主页显示的 IP 属地，随时可以取消。`
                : '用 B 站 App 扫码并确认。已经添加过的账号再扫一次，会更新它的会话。'}
            </DialogDescription>
          </DialogHeader>
          <div
            className={cn(
              'relative mx-auto grid size-[230px] place-items-center rounded-lg border bg-white p-2 transition-opacity',
              st.dim && 'opacity-30'
            )}
          >
            {img ? (
              <img src={img} alt='二维码' width={214} height={214} className='size-[214px]' />
            ) : (
              <LoaderCircle className='size-6 animate-spin text-neutral-400' />
            )}
          </div>
          <p
            role='status'
            className={cn(
              'text-center text-sm',
              st.kind === 'error' && 'text-destructive',
              st.kind === 'scanned' && 'font-medium text-success'
            )}
          >
            {st.text}
          </p>
          <DialogFooter className='sm:justify-center'>
            {st.renew && <Button onClick={() => start(target)}>换一张</Button>}
            <Button variant='outline' onClick={() => setOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </QRContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useQR() {
  const ctx = useContext(QRContext)
  if (!ctx) throw new Error('useQR 要在 QRProvider 里用')
  return ctx
}
