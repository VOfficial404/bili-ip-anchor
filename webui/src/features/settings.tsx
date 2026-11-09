import { useEffect, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ImagePlus, LoaderCircle, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api, apiJSON, Unauthorized, type LogLevel, type Settings, type VideoInfo, type Zone } from '@/lib/api'
import { fmtCount, fmtDuration, hdslbURL, zonePlace, zoneRegion } from '@/lib/format'
import { report, useRefresh, useStatusData } from '@/lib/status'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { Page } from '@/components/layout/page'

type Form = Omit<Settings, 'log_level'> & { log_level: LogLevel }

export const LEVELS: { value: LogLevel; label: string; hint: string }[] = [
  { value: 'debug', label: '调试（debug）', hint: '记录所有日志，包括每一个发给 B 站的请求，排查问题时再打开' },
  { value: 'info', label: '信息（info）', hint: '每轮运行、登录、设置变更等常规记录，推荐' },
  { value: 'warn', label: '警告（warn）', hint: '只记录警告和错误' },
  { value: 'error', label: '错误（error）', hint: '只记录错误' },
]

function durationError(s: string, minMinutes: number) {
  const m = /^(?:(\d+(?:\.\d+)?)h)?(?:(\d+(?:\.\d+)?)m)?(?:(\d+(?:\.\d+)?)s)?$/.exec(s.trim())
  if (!s.trim() || !m || (!m[1] && !m[2] && !m[3])) return '写成 90m、6h 这样'
  const min = (+m[1] || 0) * 60 + (+m[2] || 0) + (+m[3] || 0) / 60
  return min < minMinutes ? `最少 ${minMinutes}m` : ''
}

function proxyError(s: string) {
  s = s.trim()
  if (!s) return ''
  if (!/^(?:(https?|socks5h?):\/\/)?(?:[^@/\s]+@)?[^:/\s]+:\d{1,5}\/?$/.test(s))
    return '写成 http://127.0.0.1:7890 或 socks5://127.0.0.1:7891'
  return ''
}

function toForm(st: Settings): Form {
  return {
    interval: st.interval,
    jitter: st.jitter,
    proxy: st.proxy || '',
    videos: (st.videos || []).slice(),
    do_dynamic: st.do_dynamic,
    dynamic_text: st.dynamic_text || '',
    log_level: st.log_level || 'info',
  }
}

export function SettingsPage() {
  const s = useStatusData()
  const refresh = useRefresh()
  const [form, setForm] = useState<Form>(() => toForm(s.settings))
  const [edited, setEdited] = useState(false)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [videoError, setVideoError] = useState('')

  // 没改过时跟着服务端的设置走（别的窗口或客户端改了也能看到）
  useEffect(() => {
    if (!edited) setForm(toForm(s.settings))
  }, [s.settings, edited])

  const update = (patch: Partial<Form>) => {
    setForm((f) => ({ ...f, ...patch }))
    setEdited(true)
  }

  const errors = {
    interval: durationError(form.interval, 10),
    jitter: form.jitter.trim() === '0' ? '' : durationError(form.jitter, 0),
    proxy: proxyError(form.proxy),
    videos: form.videos.length ? '' : '至少要有一个心跳视频',
  }
  const invalid = Object.values(errors).some(Boolean)

  function reset() {
    setForm(toForm(s.settings))
    setEdited(false)
    setFormError('')
    setVideoError('')
  }

  async function save(e: React.FormEvent) {
    e.preventDefault()
    if (invalid) return
    setSaving(true)
    setFormError('')
    try {
      const body = {
        ...form,
        interval: form.interval.trim(),
        jitter: form.jitter.trim(),
        proxy: form.proxy.trim(),
        dynamic_text: form.dynamic_text.trim(),
      }
      const saved = await apiJSON<Settings>('PUT', '/api/settings', body)
      setForm(toForm(saved))
      setEdited(false)
      toast.success('设置已保存')
      refresh()
    } catch (err) {
      if (!(err instanceof Unauthorized)) setFormError((err as Error).message)
    }
    setSaving(false)
  }

  return (
    <Page title='设置' lead='所有账号共用，保存后立即生效。'>
      <form onSubmit={save} noValidate className='grid gap-4 pb-20'>
        <Card className='gap-5'>
          <CardHeader>
            <CardTitle>运行</CardTitle>
          </CardHeader>
          <CardContent className='grid gap-5'>
            <div className='grid gap-5 sm:grid-cols-2'>
              <Field id='interval' label='每轮间隔' error={errors.interval} hint='写成 90m、6h、12h 这样，最少 10m。'>
                <Input
                  id='interval'
                  className='font-mono'
                  spellCheck={false}
                  autoComplete='off'
                  value={form.interval}
                  aria-invalid={errors.interval ? true : undefined}
                  onChange={(e) => update({ interval: e.target.value })}
                />
              </Field>
              <Field id='jitter' label='随机抖动' error={errors.jitter} hint='每轮额外随机等 0 到这么久，让时间不那么规律。'>
                <Input
                  id='jitter'
                  className='font-mono'
                  spellCheck={false}
                  autoComplete='off'
                  value={form.jitter}
                  aria-invalid={errors.jitter ? true : undefined}
                  onChange={(e) => update({ jitter: e.target.value })}
                />
              </Field>
            </div>
            <ProxyField value={form.proxy} error={errors.proxy} onChange={(proxy) => update({ proxy })} />
            <Field
              id='log-level'
              label='日志级别'
              hint={`${LEVELS.find((l) => l.value === form.log_level)?.hint ?? ''}。调试级别会把每一个 B 站请求都记进日志，日志很快会被刷满。`}
            >
              <Select value={form.log_level} onValueChange={(v) => update({ log_level: v as LogLevel })}>
                <SelectTrigger id='log-level' className='w-full sm:w-64'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {LEVELS.map((l) => (
                    <SelectItem key={l.value} value={l.value}>
                      {l.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </CardContent>
        </Card>

        <Card className='gap-5'>
          <CardHeader>
            <CardTitle>心跳视频</CardTitle>
            <CardDescription>每轮从下面随机选一个视频上报观看心跳。任意公开视频都可以，最多 20 个。</CardDescription>
          </CardHeader>
          <CardContent className='grid gap-3'>
            <ul className='grid gap-2'>
              {form.videos.map((bvid) => (
                <VideoRow
                  key={bvid}
                  bvid={bvid}
                  canRemove={form.videos.length > 1}
                  onRemove={() => update({ videos: form.videos.filter((v) => v !== bvid) })}
                />
              ))}
            </ul>
            <AddVideos
              videos={form.videos}
              error={videoError || errors.videos}
              setError={setVideoError}
              onAdd={(list) => update({ videos: [...form.videos, ...list] })}
            />
          </CardContent>
        </Card>

        <Card className='gap-5'>
          <CardHeader>
            <CardTitle>动态</CardTitle>
          </CardHeader>
          <CardContent className='grid gap-5'>
            <div className='flex items-start justify-between gap-4'>
              <div className='grid gap-1'>
                <Label htmlFor='dynamic'>发动态</Label>
                <p className='text-xs text-muted-foreground'>
                  每轮发一条动态，几秒后删掉。只发心跳不能改变属地时再打开；动态会短暂出现在粉丝的动态流里。
                </p>
              </div>
              <Switch id='dynamic' checked={form.do_dynamic} onCheckedChange={(v) => update({ do_dynamic: v })} />
            </div>
            <Field id='dynamic-text' label='文字'>
              <Textarea
                id='dynamic-text'
                rows={3}
                maxLength={1000}
                placeholder='可以不写'
                value={form.dynamic_text}
                onChange={(e) => update({ dynamic_text: e.target.value })}
              />
            </Field>
            <Images />
            <p className='rounded-md bg-muted px-3 py-2 text-sm'>{preview(form, s.images.length)}</p>
          </CardContent>
        </Card>

        {formError && (
          <p role='alert' className='text-sm text-destructive'>
            {formError}
          </p>
        )}

        {/* 保存栏贴在底部，长页面也随时能保存 */}
        <div className='sticky bottom-0 z-10 -mx-4 flex justify-end gap-2 border-t bg-background/90 px-4 py-3 backdrop-blur'>
          <span className='me-auto self-center text-xs text-muted-foreground'>{edited ? '有未保存的修改' : ''}</span>
          <Button type='button' variant='outline' disabled={!edited || saving} onClick={reset}>
            还原
          </Button>
          <Button type='submit' disabled={!edited || invalid || saving}>
            {saving && <LoaderCircle className='animate-spin' />}
            {saving ? '正在保存' : '保存设置'}
          </Button>
        </div>
      </form>
    </Page>
  )
}

function preview(f: Form, n: number) {
  if (!f.do_dynamic) return '现在不发动态，只上报心跳。'
  const text = f.dynamic_text.trim()
  const parts: string[] = []
  if (text) parts.push(`文字「${text.length > 24 ? text.slice(0, 24) + '…' : text}」`)
  if (n) parts.push(`${n} 张图片`)
  return parts.length
    ? `每轮会发：${parts.join(' + ')}，几秒后删掉。`
    : '没有文字也没有图片时，每轮发一张内置图片，几秒后删掉。'
}

function Field({
  id,
  label,
  error,
  hint,
  children,
}: {
  id: string
  label: string
  error?: string
  hint?: string
  children: React.ReactNode
}) {
  return (
    <div className='grid content-start gap-2'>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error && <p className='text-xs text-destructive'>{error}</p>}
      {hint && <p className='text-xs text-muted-foreground'>{hint}</p>}
    </div>
  )
}

/* 代理和测试 */

function ProxyField({ value, error, onChange }: { value: string; error: string; onChange: (v: string) => void }) {
  const [result, setResult] = useState<{ text: string; tone: 'muted' | 'ok' | 'error' } | null>(null)
  const [testing, setTesting] = useState(false)

  async function test() {
    const proxy = value.trim()
    if (!proxy) {
      setResult({ text: '留空表示直连，不需要测试。', tone: 'muted' })
      return
    }
    setTesting(true)
    setResult({ text: '正在经过代理访问 B 站…', tone: 'muted' })
    try {
      const z = await apiJSON<Zone>('POST', '/api/proxy/test', { proxy })
      setResult({
        text: `可用。B 站看到的是 ${zonePlace(z)}${z.isp ? ' · ' + z.isp : ''} · ${z.addr}，主页属地会变成${zoneRegion(z)}。`,
        tone: 'ok',
      })
    } catch (err) {
      if (err instanceof Unauthorized) return
      setResult({ text: (err as Error).message, tone: 'error' })
    }
    setTesting(false)
  }

  return (
    <Field
      id='proxy'
      label='代理'
      error={error}
      hint='所有以账号身份发出的 B 站请求都经过它，B 站看到的就是代理的出口 IP。支持 http://、socks5://，例如 http://127.0.0.1:7890。'
    >
      <div className='flex gap-2'>
        <Input
          id='proxy'
          className='font-mono'
          spellCheck={false}
          autoComplete='off'
          placeholder='留空表示直连'
          value={value}
          aria-invalid={error ? true : undefined}
          onChange={(e) => {
            setResult(null)
            onChange(e.target.value)
          }}
        />
        <Button type='button' variant='outline' disabled={testing} onClick={test}>
          {testing && <LoaderCircle className='animate-spin' />}
          测试
        </Button>
      </div>
      {result && (
        <p
          role='status'
          className={cn(
            'text-xs',
            result.tone === 'muted' && 'text-muted-foreground',
            result.tone === 'ok' && 'text-success',
            result.tone === 'error' && 'text-destructive'
          )}
        >
          {result.text}
        </p>
      )}
    </Field>
  )
}

/* 心跳视频 */

type VideoResult = VideoInfo | { bvid: string; error: string }

function useVideo(bvid: string) {
  return useQuery<VideoResult>({
    queryKey: ['video', bvid],
    queryFn: async () => {
      try {
        return await apiJSON<VideoInfo>('GET', `/api/videos/${bvid}`)
      } catch (err) {
        if (err instanceof Unauthorized) throw err
        return { bvid, error: (err as Error).message }
      }
    },
    staleTime: Infinity,
    retry: false,
  })
}

function VideoRow({ bvid, canRemove, onRemove }: { bvid: string; canRemove: boolean; onRemove: () => void }) {
  const { data } = useVideo(bvid)
  const info = data && !('error' in data) ? data : null
  const cover = info ? hdslbURL(info.cover, '@160w_100h_1c.webp') : ''
  return (
    <li className='flex items-center gap-3 rounded-lg border p-2'>
      <span className='aspect-[8/5] w-20 shrink-0 overflow-hidden rounded-md bg-muted sm:w-24'>
        {cover && <img src={cover} alt='' referrerPolicy='no-referrer' className='size-full object-cover' />}
      </span>
      <div className='min-w-0 flex-1'>
        <a
          href={`https://www.bilibili.com/video/${bvid}`}
          target='_blank'
          rel='noopener noreferrer'
          className='line-clamp-1 text-sm font-medium break-all hover:underline'
          title={info?.title}
        >
          {info?.title ?? bvid}
        </a>
        <p className='text-xs text-muted-foreground'>
          <span className='font-mono'>{bvid}</span>
          {info && (
            <span>
              {' · '}
              {info.owner} · {fmtDuration(info.duration)} · {fmtCount(info.views)} 播放
            </span>
          )}
          {data && 'error' in data && <span className='text-destructive'> · {data.error}</span>}
        </p>
      </div>
      <Button
        type='button'
        variant='ghost'
        size='icon'
        aria-label='移除这个视频'
        title='移除这个视频'
        disabled={!canRemove}
        onClick={onRemove}
      >
        <Trash2 />
      </Button>
    </li>
  )
}

function AddVideos({
  videos,
  error,
  setError,
  onAdd,
}: {
  videos: string[]
  error: string
  setError: (s: string) => void
  onAdd: (list: string[]) => void
}) {
  const [text, setText] = useState('')
  const [busy, setBusy] = useState(false)

  async function add() {
    const found = [...new Set(text.match(/BV[0-9A-Za-z]{10}/g) || [])]
    if (!found.length) {
      setError('没找到 BV 号。粘贴视频链接或 BV 开头的 12 位编号')
      return
    }
    setBusy(true)
    setError('')
    const bad: string[] = []
    const ok: string[] = []
    for (const bvid of found) {
      if (videos.includes(bvid) || ok.includes(bvid)) continue
      if (videos.length + ok.length >= 20) {
        bad.push('最多 20 个视频')
        break
      }
      try {
        await apiJSON<VideoInfo>('GET', `/api/videos/${bvid}`)
        ok.push(bvid)
      } catch (err) {
        if (err instanceof Unauthorized) break
        bad.push(`${bvid}：${(err as Error).message}`)
      }
    }
    setBusy(false)
    setText('')
    if (ok.length) onAdd(ok)
    if (bad.length) setError(bad.join('；'))
  }

  return (
    <div className='grid gap-2'>
      <div className='flex gap-2'>
        <Input
          className='font-mono'
          spellCheck={false}
          autoComplete='off'
          aria-label='添加心跳视频'
          placeholder='BV 号或视频链接，可以一次粘贴多个'
          value={text}
          aria-invalid={error ? true : undefined}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              add()
            }
          }}
        />
        <Button type='button' variant='outline' disabled={busy || !text.trim()} onClick={add}>
          {busy ? <LoaderCircle className='animate-spin' /> : <Plus />}
          添加
        </Button>
      </div>
      {error && <p className='text-xs text-destructive'>{error}</p>}
    </div>
  )
}

/* 动态图片：上传和删除立即生效，不需要保存 */

function Images() {
  const { images } = useStatusData()
  const refresh = useRefresh()
  const file = useRef<HTMLInputElement>(null)
  const [uploading, setUploading] = useState(false)
  const [deleting, setDeleting] = useState('')

  async function upload(files: File[]) {
    setUploading(true)
    let n = 0
    for (const f of files) {
      try {
        await api('POST', '/api/dynamic/images', f)
        n++
      } catch (err) {
        report(err)
        if (err instanceof Unauthorized) break
      }
    }
    setUploading(false)
    if (n) toast.success(`已添加 ${n} 张图片`)
    refresh()
  }

  async function remove(name: string) {
    setDeleting(name)
    try {
      await api('DELETE', `/api/dynamic/images/${name}`)
    } catch (err) {
      report(err)
    }
    await refresh()
    setDeleting('')
  }

  return (
    <div className='grid gap-2'>
      <span className='text-sm leading-none font-medium'>图片</span>
      <ul className='grid grid-cols-3 gap-2 sm:grid-cols-5 lg:grid-cols-6'>
        {images.map((img) => (
          <li key={img.name} className='group relative aspect-square overflow-hidden rounded-lg border bg-muted'>
            <img
              src={`/api/dynamic/images/${img.name}`}
              alt='动态图片'
              loading='lazy'
              className='size-full object-cover'
            />
            <Button
              type='button'
              variant='secondary'
              size='icon'
              aria-label='删除这张图片'
              title='删除这张图片'
              disabled={deleting === img.name}
              className='absolute end-1 top-1 size-7 opacity-90 shadow-sm'
              onClick={() => remove(img.name)}
            >
              {deleting === img.name ? <LoaderCircle className='animate-spin' /> : <Trash2 />}
            </Button>
          </li>
        ))}
        {images.length < 9 && (
          <li className='aspect-square'>
            <button
              type='button'
              disabled={uploading}
              onClick={() => file.current?.click()}
              className='flex size-full flex-col items-center justify-center gap-1 rounded-lg border border-dashed text-xs text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground disabled:opacity-50'
            >
              {uploading ? <LoaderCircle className='size-5 animate-spin' /> : <ImagePlus className='size-5' />}
              {uploading ? '正在上传' : '添加图片'}
            </button>
          </li>
        )}
      </ul>
      <input
        ref={file}
        type='file'
        accept='image/jpeg,image/png,image/gif,image/webp'
        multiple
        hidden
        onChange={(e) => {
          const files = [...(e.target.files ?? [])]
          e.target.value = ''
          if (files.length) upload(files)
        }}
      />
      <p className='text-xs text-muted-foreground'>图片上传后立即生效，不需要点保存。最多 9 张，每张不超过 10 MB。</p>
    </div>
  )
}
