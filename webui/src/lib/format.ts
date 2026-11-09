import type { Status, Zone } from './api'

/* 时间显示 */

const pad = (n: number) => String(n).padStart(2, '0')

export function fmtTime(iso: string) {
  const d = new Date(iso)
  const hm = `${pad(d.getHours())}:${pad(d.getMinutes())}`
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const day = (d.getTime() - today.getTime()) / 86400000
  if (day >= 0 && day < 1) return `今天 ${hm}`
  if (day >= 1 && day < 2) return `明天 ${hm}`
  if (day < 0 && day >= -1) return `昨天 ${hm}`
  return `${d.getMonth() + 1}月${d.getDate()}日 ${hm}`
}

export function fmtDate(iso: string) {
  const d = new Date(iso)
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

// 日志里的时间：11/03 12:00:05
export function fmtLogTime(iso: string) {
  const d = new Date(iso)
  return `${pad(d.getMonth() + 1)}/${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

export function fmtRelative(iso: string) {
  const sec = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  const abs = Math.abs(sec)
  let text: string
  if (abs < 60) text = '不到 1 分钟'
  else if (abs < 3600) text = `${Math.round(abs / 60)} 分钟`
  else if (abs < 86400 * 2) {
    const h = Math.floor(abs / 3600)
    const m = Math.round((abs % 3600) / 60)
    text = m ? `${h} 小时 ${m} 分钟` : `${h} 小时`
  } else text = `${Math.floor(abs / 86400)} 天`
  return sec >= 0 ? `${text}后` : `${text}前`
}

export function fmtDuration(sec: number) {
  const m = Math.floor(sec / 60)
  return m >= 60
    ? `${Math.floor(m / 60)}:${pad(m % 60)}:${pad(sec % 60)}`
    : `${m}:${pad(sec % 60)}`
}

export function fmtCount(n: number) {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`
  return String(n)
}

/* 属地 */

export function zonePlace(z: Zone) {
  const parts = z.country === '中国' ? [z.province, z.city] : [z.country, z.province]
  return [...new Set(parts.filter(Boolean))].join(' ') || '未知地区'
}

// 主页属地显示的那一级：国内是省份，国外是国家
export function zoneRegion(z: Zone) {
  return z.country === '中国' && z.province ? z.province : z.country
}

export function whereLabel(s: Status) {
  const who = s.mode === 'local' ? '本机' : '这台服务器'
  return s.settings.proxy ? `${who}（经代理）` : who
}

/* 头像和图片 */

const AVATAR_COLORS = ['#e11d48', '#db2777', '#c026d3', '#9333ea', '#4f46e5', '#0284c7', '#0891b2', '#0d9488', '#059669', '#65a30d', '#ca8a04', '#ea580c']

export function initials(name: string) {
  const words = name.trim().split(/\s+/)
  if (words.length > 1 && /^[\x00-\x7f]+$/.test(name)) return (words[0][0] + words[1][0]).toUpperCase()
  return Array.from(name.trim())[0] || '?'
}

export function avatarColor(name: string) {
  let h = 0
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) >>> 0
  return AVATAR_COLORS[h % AVATAR_COLORS.length]
}

// hdslbURL 只接受 B 站图床的图片，加上缩略图参数
export function hdslbURL(src: string | undefined, size: string) {
  if (!/^(https?:)?\/\/[a-z0-9.-]+\.hdslb\.com\//i.test(src || '')) return ''
  return src!.replace(/^(https?:)?\/\//, 'https://') + size
}
