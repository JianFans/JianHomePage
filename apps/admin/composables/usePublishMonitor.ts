import { onScopeDispose, ref, watch, type Ref } from 'vue'
import type { AdminPublishJob } from '../utils/admin-api'

interface MonitorOptions {
  job: Ref<AdminPublishJob | null>
  active?: Readonly<Ref<boolean>>
  paused?: Readonly<Ref<boolean>>
  read: (id: string, signal: AbortSignal) => Promise<AdminPublishJob>
  latest: (signal: AbortSignal) => Promise<AdminPublishJob | null>
  updated: (job: AdminPublishJob, current: () => boolean, signal: AbortSignal) => Promise<void> | void
}

/** 只读轮询持久任务；串行请求、超时及作用域失效保护防止迟到结果覆盖新上下文。 */
export function usePublishMonitor(options: MonitorOptions) {
  const refreshing = ref(false)
  const error = ref(false)
  let disposed = false
  let syncPending = false
  let generation = 0
  let timer: ReturnType<typeof setTimeout> | undefined
  let controller: AbortController | undefined
  let deadline: ReturnType<typeof setTimeout> | undefined

  /** 清理本轮资源，退出会话或替换任务时使旧结果失效。 */
  function stop() {
    generation++
    clearTimeout(timer)
    clearTimeout(deadline)
    controller?.abort()
    refreshing.value = false
    syncPending = false
  }

  /** 不确定状态等待人工核实，其他活动任务在上一次读取完成五秒后再读。 */
  function schedule() {
    clearTimeout(timer)
    const job = options.job.value
    if (disposed || options.active?.value === false || refreshing.value || !job) return
    const active = ['pending', 'building'].includes(job.status)
    if (!active && !syncPending) return
    const buildEnded = job.build?.phase === 'succeeded' || job.build?.phase === 'failed'
    if (active && !buildEnded && (job.build?.configurationMismatch || (job.build?.phase === 'submitting' && job.build.error))) return
    timer = setTimeout(() => { void refresh() }, 5000)
  }

  /** 恢复或刷新仅获取状态；不创建部署，不弹 Toast，也不阻塞内容写操作。 */
  async function fetchStatus(latest: boolean) {
    if (disposed || options.active?.value === false || refreshing.value) return
    if (options.paused?.value) { schedule(); return }
    if (!latest && !options.job.value) return
    clearTimeout(timer)
    const sequence = ++generation
    let expectedID = options.job.value?.id
    const request = new AbortController()
    controller = request
    const timeout = setTimeout(() => request.abort(), 15000)
    deadline = timeout
    refreshing.value = true
    /** 每个异步边界都验证身份，不能让旧请求修改新任务或已退出会话。 */
    const valid = () => !disposed && options.active?.value !== false && sequence === generation && expectedID === options.job.value?.id
    const current = () => valid() && !options.paused?.value && !request.signal.aborted
    try {
      const job = latest ? await options.latest(request.signal) : await options.read(expectedID!, request.signal)
      if (!current()) return
      options.job.value = job
      expectedID = job?.id
      error.value = false
      syncPending = job?.status === 'succeeded' || job?.status === 'failed'
      if (job) {
        await options.updated(job, current, request.signal)
        if (current()) syncPending = false
      }
    } catch {
      if (valid() && !options.paused?.value) error.value = true
    } finally {
      clearTimeout(timeout)
      if (sequence === generation) { refreshing.value = false; schedule() }
    }
  }

  /** 手动刷新复用同一串行读取通道。 */
  async function refresh() { await fetchStatus(false) }
  /** 初次进入或显式重试时恢复服务端最新任务。 */
  async function restore() { await fetchStatus(true) }
  watch(() => [options.job.value?.id, options.job.value?.status, options.job.value?.build?.phase, options.job.value?.build?.configurationMismatch, options.job.value?.build?.error], schedule, { flush: 'sync', immediate: true })
  if (options.paused) watch(options.paused, paused => { if (!paused) schedule() }, { flush: 'sync' })
  if (options.active) watch(options.active, active => { if (!active) stop(); else schedule() }, { flush: 'sync' })
  onScopeDispose(() => { disposed = true; stop() })
  return { refreshing, error, refresh, restore, stop }
}
