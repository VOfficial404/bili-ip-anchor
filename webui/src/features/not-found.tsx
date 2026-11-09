import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'

export function NotFound() {
  return (
    <div className='flex min-h-svh w-full flex-col items-center justify-center gap-2 p-6 text-center'>
      <h1 className='text-[7rem] leading-tight font-bold'>404</h1>
      <span className='font-medium'>没有这个页面</span>
      <p className='text-muted-foreground'>地址可能写错了，或者这个页面已经不在了。</p>
      <Button asChild className='mt-6'>
        <Link to='/'>回到概览</Link>
      </Button>
    </div>
  )
}
