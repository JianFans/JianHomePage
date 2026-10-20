import { onScopeDispose, readonly, ref } from 'vue'
import { createEntityId } from '../utils/identity'

export type ToastTone = 'info' | 'success' | 'warning' | 'error'
export interface ToastMessage { id: string; message: string; tone: ToastTone }
interface Lifetime { remaining: number; started: number; timer?: ReturnType<typeof setTimeout>; pauses: Set<string> }

/** 管理独立的消息队列；暂停原因分别计数，防止指针移出恢复仍在聚焦的消息。 */
export function useToasts() {
  const items = ref<ToastMessage[]>([])
  const queue: ToastMessage[] = []
  const lifetimes = new Map<string, Lifetime>()
  const globalPauses = new Set<string>()
  let disposed = false

  /** 只为当前可见且没有暂停原因的消息启动剩余计时。 */
  function start(id: string) {
    const life = lifetimes.get(id)
    if (!life || life.timer !== undefined || life.pauses.size || globalPauses.size || disposed) return
    life.started = Date.now()
    life.timer = setTimeout(() => dismiss(id), life.remaining)
  }

  /** 取消计时并记住剩余时间，不把暂停期间算入展示时间。 */
  function stop(life: Lifetime) {
    if (life.timer === undefined) return
    clearTimeout(life.timer)
    life.timer = undefined
    life.remaining = Math.max(0, life.remaining - (Date.now() - life.started))
  }

  /** 按到达顺序补位；队列中的消息尚未开始消耗展示时间。 */
  function fill() {
    while (!disposed && items.value.length < 3 && queue.length) {
      const item = queue.shift()!
      items.value.push(item)
      start(item.id)
    }
  }

  /** 创建文本消息，错误提示保留更长阅读时间。 */
  function notify(message: string, tone: ToastTone = 'info'): string {
    const id = createEntityId('toast_')
    if (disposed) return id
    lifetimes.set(id, { remaining: tone === 'error' ? 7000 : 5000, started: 0, pauses: new Set() })
    queue.push({ id, message, tone })
    fill()
    return id
  }

  /** 关闭指定消息，并释放其计时资源和队列位置。 */
  function dismiss(id: string) {
    const life = lifetimes.get(id)
    if (life) stop(life)
    lifetimes.delete(id)
    items.value = items.value.filter(item => item.id !== id)
    const queued = queue.findIndex(item => item.id === id)
    if (queued >= 0) queue.splice(queued, 1)
    fill()
  }

  /** 每种交互独立暂停，重复进入同一原因不重置时间。 */
  function pause(id: string, reason: string) {
    const life = lifetimes.get(id)
    if (!life) return
    stop(life)
    life.pauses.add(reason)
  }

  /** 只有最后一个暂停原因结束时才恢复可见消息。 */
  function resume(id: string, reason: string) {
    lifetimes.get(id)?.pauses.delete(reason)
    if (items.value.some(item => item.id === id)) start(id)
  }

  /** 页面隐藏等全局原因也适用于暂停期间新加入的消息。 */
  function pauseAll(reason: string) {
    globalPauses.add(reason)
    for (const life of lifetimes.values()) stop(life)
  }

  /** 恢复全局暂停后只启动可见消息。 */
  function resumeAll(reason: string) {
    globalPauses.delete(reason)
    for (const item of items.value) start(item.id)
  }

  onScopeDispose(() => {
    disposed = true
    for (const life of lifetimes.values()) stop(life)
    lifetimes.clear()
    queue.length = 0
    items.value = []
  })
  return { items: readonly(items), notify, dismiss, pause, resume, pauseAll, resumeAll }
}
