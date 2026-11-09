import { Play, Plus } from 'lucide-react'
import { useRunAll, useStatus } from '@/lib/status'
import { useQR } from '@/context/qr-provider'
import { Button } from '@/components/ui/button'
import { Search } from '@/components/search'
import { ThemeSwitch } from '@/components/theme-switch'
import { Header } from './header'
import { Main } from './main'

type PageProps = {
  title: string
  lead?: React.ReactNode
  actions?: React.ReactNode
  fixed?: boolean
  children: React.ReactNode
}

// Page 是每个页面的外框：顶栏（搜索、主题）、标题、右侧按钮和正文
export function Page({ title, lead, actions, fixed, children }: PageProps) {
  const { isError, error, data } = useStatus()
  return (
    <>
      <Header fixed>
        <Search className='me-auto md:me-0 md:ms-auto' />
        <ThemeSwitch />
      </Header>
      <Main id='content' fixed={fixed} className='flex flex-col gap-4'>
        <div className='flex flex-wrap items-end justify-between gap-x-4 gap-y-2'>
          <div className='min-w-0 space-y-1'>
            <h1 className='text-2xl font-bold tracking-tight'>{title}</h1>
            {lead && <p className='text-sm text-muted-foreground'>{lead}</p>}
          </div>
          {actions && <div className='flex flex-wrap items-center gap-2'>{actions}</div>}
        </div>
        {isError && data && (
          <p
            role='alert'
            className='rounded-lg border border-warning/40 bg-warning/10 px-4 py-2.5 text-sm text-warning'
          >
            连不上服务，显示的是之前的数据：{error.message}
          </p>
        )}
        {children}
      </Main>
    </>
  )
}

// AccountActions 是概览和账号页右上角的「全部立即运行」「添加账号」
export function AccountActions() {
  const { data } = useStatus()
  const openQR = useQR()
  const runAll = useRunAll()
  const list = data?.accounts ?? []
  return (
    <>
      {list.length > 0 && (
        <Button
          variant='outline'
          disabled={!list.some((a) => a.logged_in && !a.running && !a.busy)}
          onClick={runAll}
        >
          <Play />
          全部立即运行
        </Button>
      )}
      <Button onClick={() => openQR()}>
        <Plus />
        添加账号
      </Button>
    </>
  )
}
