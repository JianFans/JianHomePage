import { onScopeDispose, readonly, ref } from 'vue'
import { createEntityId } from '../utils/identity'

export type ToastTone = 'info' | 'success' | 'warning' | 'error'
export interface ToastMessage { id: string; message: string; tone: ToastTone }
interface Lifetime { remaining: number; started: number; timer?: ReturnType<typeof setTimeout>; pauses: Set<string> }

/** 最新提示立即可见，超过三条时淘汰最旧提示；每条独立维护暂停原因。 */
export function useToasts() {
  const items = ref<ToastMessage[]>([])
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

  /** 创建文本消息，错误提示保留更长阅读时间。 */
  function notify(message: string, tone: ToastTone = 'info'): string {
    const id = createEntityId('toast_')
    if (disposed) return id
    lifetimes.set(id, { remaining: tone === 'error' ? 7000 : 5000, started: 0, pauses: new Set() })
    if (items.value.length >= 3) dismiss(items.value.at(-1)!.id)
    items.value.unshift({ id, message, tone })
    start(id)
    return id
  }

  /** 关闭或淘汰指定消息并释放计时资源，迟到交互不会恢复已删除的提示。 */
  function dismiss(id: string) {
    const life = lifetimes.get(id)
    if (life) stop(life)
    lifetimes.delete(id)
    items.value = items.value.filter(item => item.id !== id)
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
    items.value = []
  })
  return { items: readonly(items), notify, dismiss, pause, resume, pauseAll, resumeAll }
}
