import { useRef, useState } from 'react'
import { Download, FolderHeart, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { api, type Account, type FavImport } from '@/lib/api'
import { fmtTime } from '@/lib/format'
import { report, useRefresh } from '@/lib/status'
import { cn } from '@/lib/utils'
import { useConfirm } from '@/context/confirm-provider'
import { Button } from '@/components/ui/button'

type FavFile = {
  format: string
  version: number
  uname: string
  mid: string
  folders: { title: string; videos: { invalid?: boolean }[] }[]
}

// FavoritesRow 是账号卡片底部的「收藏夹」一栏：导出成文件、从文件导入、导入进度
export function FavoritesRow({ a, busy }: { a: Account; busy: boolean }) {
  const [pending, setPending] = useState<'' | 'export' | 'import'>('')
  const input = useRef<HTMLInputElement>(null)
  const confirm = useConfirm()
  const refresh = useRefresh()
  const label = a.uname || `UID ${a.mid}`
  const path = `/api/accounts/${a.mid}/favorites`
  const job = a.fav_import

  async function exportFile() {
    setPending('export')
    const id = toast.loading(`正在读取「${label}」的收藏夹…`)
    try {
      const res = await api('GET', path)
      const name =
        /filename="([^"]+)"/.exec(res.headers.get('Content-Disposition') ?? '')?.[1] ??
        `bili-favorites-${a.mid}.json`
      const blob = await res.blob()
      const link = document.createElement('a')
      link.href = URL.createObjectURL(blob)
      link.download = name
      link.click()
      setTimeout(() => URL.revokeObjectURL(link.href), 10_000)
      toast.success(`已导出 ${name}`, { id })
    } catch (err) {
      toast.dismiss(id)
      report(err)
    }
    setPending('')
  }

  async function pick(file: File) {
    let x: FavFile
    try {
      x = JSON.parse(await file.text())
      if (x.format !== 'bili-ip-anchor/favorites' || !Array.isArray(x.folders)) throw new Error()
    } catch {
      toast.error('不是 bili-ip-anchor 导出的收藏夹文件')
      return
    }
    const videos = x.folders.reduce((n, f) => n + f.videos.filter((v) => !v.invalid).length, 0)
    const minutes = Math.max(1, Math.round((videos * 2.5) / 60))
    confirm({
      title: `导入到「${label}」？`,
      desc:
        `来自「${x.uname || x.mid}」的 ${x.folders.length} 个收藏夹、${videos} 个视频。` +
        `同名的收藏夹会合并，没有的会新建；已经收藏的视频跳过。` +
        `为了不被 B 站限制，每个视频间隔两三秒，大约要 ${minutes} 分钟，期间这个账号不会运行定时任务。`,
      confirmText: '开始导入',
      action: async () => {
        setPending('import')
        try {
          await api('POST', path, new Blob([await file.arrayBuffer()], { type: 'application/json' }))
          toast.success('开始导入，进度显示在账号卡片上')
        } catch (err) {
          report(err)
        }
        setPending('')
        refresh()
      },
    })
  }

  return (
    <div className='flex flex-col gap-3 border-t px-4 py-3 sm:px-5'>
      <div className='flex flex-wrap items-center gap-x-3 gap-y-2'>
        <p className='flex min-w-0 flex-1 basis-56 items-center gap-1.5 text-xs font-medium text-muted-foreground'>
          <FolderHeart className='size-3.5' />
          收藏夹
          <span className='font-normal'>· 导出成文件，可以导入到这个或别的账号</span>
        </p>
        <div className='flex gap-2'>
          <Button variant='outline' size='sm' disabled={busy || !a.logged_in || pending !== ''} onClick={exportFile}>
            <Download />
            {pending === 'export' ? '正在导出' : '导出'}
          </Button>
          <Button
            variant='outline'
            size='sm'
            disabled={busy || !a.logged_in || pending !== ''}
            onClick={() => input.current?.click()}
          >
            <Upload />
            导入
          </Button>
          <input
            ref={input}
            type='file'
            accept='application/json,.json'
            hidden
            onChange={(e) => {
              const f = e.target.files?.[0]
              e.target.value = ''
              if (f) pick(f)
            }}
          />
        </div>
      </div>
      {job && <ImportProgress job={job} />}
    </div>
  )
}

function ImportProgress({ job }: { job: FavImport }) {
  const pct = job.total > 0 ? Math.round((job.done / job.total) * 100) : 100
  let head: string
  if (job.running) head = job.folder ? `正在导入「${job.folder}」` : '正在导入'
  else if (job.error) head = '导入中途停下'
  else head = '导入完成'
  const counts = [`新增 ${job.added}`, `已有 ${job.skipped}`]
  if (job.failed) counts.push(`失败 ${job.failed}`)
  if (job.created?.length) counts.push(`新建了 ${job.created.map((t) => `「${t}」`).join('')}`)
  return (
    <div className='grid gap-1.5 text-xs' aria-live='polite'>
      <div className='flex flex-wrap justify-between gap-x-3'>
        <span className={cn('font-medium', job.error && 'text-destructive')}>{head}</span>
        <span className='text-muted-foreground tabular-nums'>
          {job.done}/{job.total}
          {job.finished && ` · ${fmtTime(job.finished)}`}
        </span>
      </div>
      <div
        className='h-1.5 overflow-hidden rounded-full bg-muted'
        role='progressbar'
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
      >
        <div
          className={cn('h-full rounded-full bg-primary transition-[width]', job.error && 'bg-destructive')}
          style={{ width: `${pct}%` }}
        />
      </div>
      <p className='text-muted-foreground'>{counts.join(' · ')}</p>
      {job.error && <p className='break-words text-destructive'>{job.error}</p>}
      {job.failures && job.failures.length > 0 && (
        <details className='text-muted-foreground'>
          <summary className='cursor-pointer'>没有收藏上的视频</summary>
          <ul className='mt-1 grid gap-0.5'>
            {job.failures.map((f) => (
              <li key={f.bvid} className='break-words'>
                <a
                  className='font-mono underline-offset-2 hover:underline'
                  href={`https://www.bilibili.com/video/${f.bvid}`}
                  target='_blank'
                  rel='noreferrer'
                >
                  {f.bvid}
                </a>{' '}
                {f.title} — {f.reason}
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  )
}
