import React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ArrowRight, Laptop, Lock, Moon, Play, Plus, Sun } from 'lucide-react'
import { useLock, useRunAll, useStatusData } from '@/lib/status'
import { useQR } from '@/context/qr-provider'
import { useSearch } from '@/context/search-provider'
import { useTheme } from '@/context/theme-provider'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { navItems } from './layout/nav'

export function CommandMenu() {
  const navigate = useNavigate()
  const { setTheme } = useTheme()
  const { open, setOpen } = useSearch()
  const status = useStatusData()
  const openQR = useQR()
  const runAll = useRunAll()
  const lock = useLock()

  const runCommand = React.useCallback(
    (command: () => unknown) => {
      setOpen(false)
      command()
    },
    [setOpen]
  )

  const canRun = status.accounts.some((a) => a.logged_in && !a.running && !a.busy)

  return (
    <CommandDialog modal open={open} onOpenChange={setOpen}>
      <CommandInput placeholder='搜索页面或操作…' />
      <CommandList className='max-h-80'>
        <CommandEmpty>没有找到</CommandEmpty>
        <CommandGroup heading='页面'>
          {navItems.map((item) => (
            <CommandItem
              key={item.url}
              value={`页面 ${item.title}`}
              onSelect={() => runCommand(() => navigate({ to: item.url }))}
            >
              <div className='flex size-4 items-center justify-center'>
                <ArrowRight className='size-2 text-muted-foreground/80' />
              </div>
              {item.title}
            </CommandItem>
          ))}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading='操作'>
          <CommandItem value='添加账号 扫码' onSelect={() => runCommand(() => openQR())}>
            <Plus />
            <span>添加账号</span>
          </CommandItem>
          {canRun && (
            <CommandItem value='全部立即运行' onSelect={() => runCommand(runAll)}>
              <Play />
              <span>全部立即运行</span>
            </CommandItem>
          )}
          {status.mode !== 'local' && (
            <CommandItem value='锁定面板 退出' onSelect={() => runCommand(lock)}>
              <Lock />
              <span>锁定面板</span>
            </CommandItem>
          )}
        </CommandGroup>
        <CommandSeparator />
        <CommandGroup heading='主题'>
          <CommandItem value='主题 浅色' onSelect={() => runCommand(() => setTheme('light'))}>
            <Sun /> <span>浅色</span>
          </CommandItem>
          <CommandItem value='主题 深色' onSelect={() => runCommand(() => setTheme('dark'))}>
            <Moon className='scale-90' />
            <span>深色</span>
          </CommandItem>
          <CommandItem value='主题 跟随系统' onSelect={() => runCommand(() => setTheme('system'))}>
            <Laptop />
            <span>跟随系统</span>
          </CommandItem>
        </CommandGroup>
      </CommandList>
    </CommandDialog>
  )
}
