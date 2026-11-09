import { LayoutDashboard, ScrollText, SlidersHorizontal, Users } from 'lucide-react'
import type { PagePath } from '@/router'

export const navItems: { title: string; url: PagePath; icon: React.ElementType }[] = [
  { title: '概览', url: '/', icon: LayoutDashboard },
  { title: '账号', url: '/accounts', icon: Users },
  { title: '设置', url: '/settings', icon: SlidersHorizontal },
  { title: '日志', url: '/logs', icon: ScrollText },
]
