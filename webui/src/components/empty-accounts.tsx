import { Users } from 'lucide-react'
import { useQR } from '@/context/qr-provider'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'

export function EmptyAccounts() {
  const openQR = useQR()
  return (
    <Card>
      <CardContent className='flex flex-col items-center gap-2 py-6 text-center'>
        <Users className='size-10 text-muted-foreground' />
        <p className='font-semibold'>还没有账号</p>
        <p className='max-w-md text-sm text-muted-foreground'>
          扫码登录一个 B 站账号，它就会按设置定时运行。可以添加多个，每个账号各自排程、同时进行。
        </p>
        <Button className='mt-2' onClick={() => openQR()}>
          扫码添加第一个账号
        </Button>
      </CardContent>
    </Card>
  )
}
