import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Anchor, LoaderCircle } from 'lucide-react'
import { api, setAuth } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { PasswordInput } from '@/components/password-input'
import { ThemeSwitch } from '@/components/theme-switch'

export function Login() {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const qc = useQueryClient()

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError('')
    setBusy(true)
    try {
      await api('POST', '/api/session', { password })
      setPassword('')
      await qc.resetQueries({ queryKey: ['status'] })
      setAuth('in')
    } catch (err) {
      const msg = (err as Error).message
      setError(msg === '密码不对' ? '面板密码不对' : msg)
    }
    setBusy(false)
  }

  return (
    <div className='relative grid min-h-svh place-items-center p-4'>
      <div className='absolute end-4 top-4'>
        <ThemeSwitch />
      </div>
      <div className='flex w-full max-w-sm flex-col gap-4'>
        <div className='flex items-center justify-center gap-2'>
          <div className='flex size-8 items-center justify-center rounded-lg bg-brand text-white'>
            <Anchor className='size-4' />
          </div>
          <h1 className='text-xl font-medium'>Bili IP Anchor</h1>
        </div>
        <Card className='gap-4'>
          <CardHeader>
            <CardTitle className='text-lg tracking-tight'>进入面板</CardTitle>
            <CardDescription>
              输入面板密码，就是启动时 <code className='font-mono'>WEBUI_PASSWORD</code> 里设置的那个。
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form onSubmit={submit} className='grid gap-3'>
              <div className='grid gap-2'>
                <Label htmlFor='password'>面板密码</Label>
                <PasswordInput
                  id='password'
                  autoComplete='current-password'
                  required
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  aria-invalid={error ? true : undefined}
                />
              </div>
              <p role='alert' className='min-h-5 text-sm text-destructive'>
                {error}
              </p>
              <Button type='submit' disabled={busy || !password}>
                {busy && <LoaderCircle className='animate-spin' />}
                进入面板
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
