import { useSyncExternalStore } from 'react'

export type Zone = {
  addr: string
  country: string
  province: string
  city: string
  isp: string
}

export type LogLevel = 'debug' | 'info' | 'warn' | 'error'

export type Settings = {
  videos: string[]
  do_dynamic: boolean
  dynamic_text: string
  interval: string
  jitter: string
  proxy: string
  log_level: LogLevel
}

export type HomeLocation = {
  text: string
  checked_at: string
  matched_since?: string
  error?: string
}

export type Account = {
  mid: string
  uname: string
  face: string
  logged_in: boolean
  running: boolean
  busy?: string
  last: { at: string; ok: boolean; error?: string } | null
  last_ok: string | null
  next: string | null
  stats: {
    coins: number
    bcoins: number
    level: number
    vip?: string
    updated_at: string
  } | null
  cookie: { expires?: string; can_refresh: boolean }
  app_auth: boolean
  home: HomeLocation | null
  fav_import?: FavImport
}

/* 导入收藏的进度，见 core/favorites.go */
export type FavImport = {
  running: boolean
  folder?: string
  total: number
  done: number
  added: number
  skipped: number
  failed: number
  failures?: { bvid: string; title?: string; reason: string }[]
  created?: string[]
  error?: string
  started: string
  finished?: string
}

export type ImageInfo = { name: string; size: number }

export type Status = {
  version: string
  mode: 'server' | 'local'
  accounts: Account[]
  zone: Zone | null
  zone_error?: string
  zone_checked_at?: string
  settings: Settings
  images: ImageInfo[]
  log_level: LogLevel
}

export type LogEntry = {
  seq: number
  time: string
  level: LogLevel
  mid?: string
  account?: string
  msg: string
}

export type VideoInfo = {
  bvid: string
  title: string
  cover: string
  owner: string
  duration: number
  views: number
}

export class Unauthorized extends Error {}

/* 面板是否已输入密码。unknown 是刚打开、还没问过服务端 */

type AuthState = 'unknown' | 'in' | 'out'
let authState: AuthState = 'unknown'
const authListeners = new Set<() => void>()

export function setAuth(s: AuthState) {
  if (s === authState) return
  authState = s
  authListeners.forEach((fn) => fn())
}

export function useAuth() {
  return useSyncExternalStore(
    (fn) => {
      authListeners.add(fn)
      return () => authListeners.delete(fn)
    },
    () => authState
  )
}

// 非 GET 请求都要带 X-Requested-With（服务端用它防 CSRF）。
// body 是对象时按 JSON 发；是 Blob（上传图片）时原样发。
export async function api(method: string, path: string, body?: unknown) {
  const headers: Record<string, string> = { 'X-Requested-With': 'bia' }
  let payload: BodyInit | undefined
  if (body instanceof Blob) {
    if (body.type) headers['Content-Type'] = body.type
    payload = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    payload = JSON.stringify(body)
  }
  const res = await fetch(path, {
    method,
    headers,
    body: payload,
    credentials: 'same-origin',
  })
  if (res.status === 401 && path !== '/api/session') {
    setAuth('out')
    throw new Unauthorized('请先输入密码')
  }
  if (!res.ok) {
    let msg = `请求失败（HTTP ${res.status}）`
    try {
      msg = (await res.json()).error || msg
    } catch {
      /* 不是 JSON */
    }
    throw new Error(msg)
  }
  return res
}

export async function apiJSON<T>(method: string, path: string, body?: unknown) {
  return (await (await api(method, path, body)).json()) as T
}
