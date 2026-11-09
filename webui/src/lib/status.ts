import { useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api, apiJSON, setAuth, Unauthorized, type Status } from './api'

// useStatus 每 5 秒拉一次 /api/status，所有页面共用这份数据
export function useStatus(enabled = true) {
  return useQuery({
    queryKey: ['status'],
    queryFn: () => apiJSON<Status>('GET', '/api/status'),
    enabled,
    refetchInterval: (q) => (q.state.error instanceof Unauthorized ? false : 5000),
    retry: (n, err) => !(err instanceof Unauthorized) && n < 1,
  })
}

// useStatusData 只在面板里用，那时一定已经拿到过状态
export function useStatusData() {
  return useStatus().data!
}

export function useRefresh() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['status'] })
}

// report 把错误显示成提示，面板被锁时不提示
export function report(err: unknown) {
  if (err instanceof Unauthorized) return
  toast.error(err instanceof Error ? err.message : String(err))
}

export function useRunAll() {
  const refresh = useRefresh()
  return async () => {
    try {
      const { triggered } = await apiJSON<{ triggered: number }>('POST', '/api/run')
      toast.success(`已让 ${triggered} 个账号立即运行`)
    } catch (err) {
      report(err)
    }
    setTimeout(refresh, 600)
  }
}

export function useLock() {
  const qc = useQueryClient()
  return async () => {
    try {
      await api('DELETE', '/api/session')
    } catch {
      /* 会话本来就失效了 */
    }
    setAuth('out')
    qc.removeQueries()
  }
}
