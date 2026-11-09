import { createContext, useCallback, useContext, useState } from 'react'
import { ConfirmDialog } from '@/components/confirm-dialog'

type Ask = {
  title: string
  desc: string
  confirmText: string
  destructive?: boolean
  action: () => void
}

const ConfirmContext = createContext<((a: Ask) => void) | null>(null)

// ConfirmProvider 提供一个共用的确认对话框，用 useConfirm() 打开
export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const [ask, setAsk] = useState<Ask | null>(null)
  const [open, setOpen] = useState(false)
  const show = useCallback((a: Ask) => {
    setAsk(a)
    setOpen(true)
  }, [])
  return (
    <ConfirmContext value={show}>
      {children}
      {ask && (
        <ConfirmDialog
          open={open}
          onOpenChange={setOpen}
          title={ask.title}
          desc={ask.desc}
          confirmText={ask.confirmText}
          destructive={ask.destructive}
          handleConfirm={() => {
            setOpen(false)
            ask.action()
          }}
        />
      )}
    </ConfirmContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export function useConfirm() {
  const ctx = useContext(ConfirmContext)
  if (!ctx) throw new Error('useConfirm 要在 ConfirmProvider 里用')
  return ctx
}
