import { effectScope, ref } from 'vue'
import { afterEach, expect, it, vi } from 'vitest'
import { usePublishMonitor } from '../../composables/usePublishMonitor'
import type { AdminPublishJob } from '../../utils/admin-api'

afterEach(() => vi.useRealTimers())

/** A controllable read models a slow network without replacing the monitor's lifecycle. */
function pending<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((done) => { resolve = done })
  return { promise, resolve }
}

/** Realistically shaped task data keeps monitoring separate from cloud mutations. */
function task(status: AdminPublishJob['status'] = 'building'): AdminPublishJob {
  return { id: 'pub_one', versionId: 'ver_one', status, snapshotKey: 'snapshot', snapshotChecksum: 'sha256:test' }
}

it('自动读取最新状态，不重叠请求，终态停止轮询', async () => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>(task())
  const slow = pending<AdminPublishJob>()
  const get = vi.fn().mockReturnValueOnce(slow.promise).mockResolvedValue(task('succeeded'))
  const update = vi.fn()
  const monitor = scope.run(() => usePublishMonitor({ job, read: get, latest: async () => null, updated: update }))!
  await vi.advanceTimersByTimeAsync(5000)
  expect(monitor.refreshing.value).toBe(true)
  await vi.advanceTimersByTimeAsync(5000)
  expect(get).toHaveBeenCalledTimes(1)
  slow.resolve(task())
  await vi.advanceTimersByTimeAsync(0)
  await vi.advanceTimersByTimeAsync(5000)
  expect(job.value?.status).toBe('succeeded')
  await vi.advanceTimersByTimeAsync(20000)
  expect(get).toHaveBeenCalledTimes(2)
  scope.stop()
  expect(vi.getTimerCount()).toBe(0)
})

it.each(['succeeded', 'failed'] as const)('配置变化不阻止已结束构建 %s 的领域状态同步', async (phase) => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>({ ...task(), build: { phase, configurationMismatch: true } })
  const read = vi.fn().mockResolvedValue({ ...task(phase), build: { phase, configurationMismatch: true } })
  scope.run(() => usePublishMonitor({ job, read, latest: async () => null, updated: async () => {} }))
  try {
    await vi.advanceTimersByTimeAsync(5000)
    expect(read).toHaveBeenCalledTimes(1)
    expect(job.value?.status).toBe(phase)
    await vi.advanceTimersByTimeAsync(15000)
    expect(read).toHaveBeenCalledTimes(1)
  } finally { scope.stop() }
})

it.each(['succeeded', 'failed'] as const)('配置变化后已部署任务持续轮询直到 %s', async (phase) => {
  vi.useFakeTimers()
  const scope = effectScope()
  const deployed: AdminPublishJob = { ...task(), build: { phase: 'deployed', deploymentId: 'dp-original', configurationMismatch: true } }
  const job = ref<AdminPublishJob | null>(deployed)
  let reads = 0
  const read = vi.fn(async () => ++reads === 1 ? deployed : { ...task(phase), build: { phase, configurationMismatch: true } })
  let synchronized: AdminPublishJob['status'] | undefined
  scope.run(() => usePublishMonitor({ job, read, latest: async () => null, updated: result => { synchronized = result.status } }))
  try {
    await vi.advanceTimersByTimeAsync(5000)
    expect(synchronized).toBe('building')
    expect(job.value?.build?.deploymentId).toBe('dp-original')
    await vi.advanceTimersByTimeAsync(5000)
    expect(job.value?.status).toBe(phase)
    expect(synchronized).toBe(phase)
    await vi.advanceTimersByTimeAsync(15000)
    expect(read).toHaveBeenCalledTimes(2)
  } finally { scope.stop() }
})

it.each(['queued', 'building', 'submitting'] as const)('活动构建 %s 的配置变化仍暂停自动读取', async (phase) => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>({ ...task(), build: { phase, configurationMismatch: true } })
  const read = vi.fn()
  scope.run(() => usePublishMonitor({ job, read, latest: async () => null, updated: async () => {} }))
  try {
    await vi.advanceTimersByTimeAsync(20000)
    expect(read).not.toHaveBeenCalled()
  } finally { scope.stop() }
})

it('恢复持久任务；退出会话后迟到响应不能写回，保留错误并允许重试', async () => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>(null)
  const active = ref(true)
  const slow = pending<AdminPublishJob>()
  const latest = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(task())
  const monitor = scope.run(() => usePublishMonitor({ job, active, latest, read: () => slow.promise, updated: async () => {} }))!
  await monitor.restore()
  expect(monitor.error.value).toBe(true)
  await monitor.restore()
  expect(job.value?.id).toBe('pub_one')
  expect(monitor.error.value).toBe(false)
  const refreshing = monitor.refresh()
  active.value = false
  slow.resolve(task('succeeded'))
  await refreshing
  expect(job.value?.status).toBe('building')
  scope.stop()
  expect(vi.getTimerCount()).toBe(0)
})

it('历史响应不覆盖替换后的任务，也不在编辑写操作期间发起轮询', async () => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>(task())
  const paused = ref(true)
  const slow = pending<AdminPublishJob>()
  const read = vi.fn().mockReturnValue(slow.promise)
  const monitor = scope.run(() => usePublishMonitor({ job, paused, read, latest: async () => null, updated: async () => {} }))!
  await vi.advanceTimersByTimeAsync(5000)
  expect(read).not.toHaveBeenCalled()
  paused.value = false
  const refreshing = monitor.refresh()
  job.value = { ...task(), id: 'pub_new' }
  slow.resolve(task('failed'))
  await refreshing
  expect(job.value.id).toBe('pub_new')
  expect(job.value.status).toBe('building')
  scope.stop()
})

it('终态后的版本同步也受同一次超时限制，失败可再次刷新', async () => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>(null)
  let syncSignal: AbortSignal | undefined
  const monitor = scope.run(() => usePublishMonitor({ job, latest: async () => task('succeeded'), read: async () => task('succeeded'),
    updated: async (_job, _current, signal) => {
      syncSignal = signal
      return new Promise<void>((_resolve, reject) => {
        signal?.addEventListener('abort', () => reject(new Error('Timed out')), { once: true })
      })
    },
  }))!
  try {
    const restoring = monitor.restore()
    await vi.advanceTimersByTimeAsync(0)
    expect(syncSignal).toBeInstanceOf(AbortSignal)
    expect(monitor.refreshing.value).toBe(true)
    await vi.advanceTimersByTimeAsync(15000)
    await restoring
    expect(monitor.refreshing.value).toBe(false)
    expect(monitor.error.value).toBe(true)
    expect(job.value?.status).toBe('succeeded')
  } finally { scope.stop() }
})

it('终态同步被写操作打断后，解除暂停会补读直到关联版本同步完成', async () => {
  vi.useFakeTimers()
  const scope = effectScope()
  const job = ref<AdminPublishJob | null>(task())
  const paused = ref(false)
  const versionRead = pending<void>()
  let synchronized = false
  let reads = 0
  const monitor = scope.run(() => usePublishMonitor({ job, paused, latest: async () => null, read: async () => task('succeeded'),
    updated: async (_job, current) => {
      if (++reads === 1) await versionRead.promise
      if (current()) synchronized = true
    },
  }))!
  try {
    const refreshing = monitor.refresh()
    await vi.advanceTimersByTimeAsync(0)
    paused.value = true
    versionRead.resolve()
    await refreshing
    expect(synchronized).toBe(false)
    paused.value = false
    await vi.advanceTimersByTimeAsync(5000)
    expect(synchronized).toBe(true)
    expect(monitor.error.value).toBe(false)
  } finally { scope.stop() }
})
