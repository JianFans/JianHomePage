import { effectScope } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useToasts } from '../../composables/useToasts'

afterEach(() => { vi.useRealTimers() })

describe('右下角提示生命周期', () => {
  it('新消息立即替换最旧消息，旧消息不会在关闭后补回', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const toasts = scope.run(() => useToasts())!
    const ids = ['甲', '乙', '丙', '丁'].map(message => toasts.notify(message))
    expect(ids.every(id => /^toast_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id))).toBe(true)
    expect(new Set(ids).size).toBe(ids.length)
    expect(toasts.items.value.map(item => item.message)).toEqual(['丁', '丙', '乙'])
    expect(vi.getTimerCount()).toBe(3)
    toasts.dismiss(ids[0]!)
    expect(toasts.items.value.map(item => item.message)).toEqual(['丁', '丙', '乙'])
    toasts.dismiss(ids[1]!)
    expect(toasts.items.value.map(item => item.message)).toEqual(['丁', '丙'])
    scope.stop()
    expect(vi.getTimerCount()).toBe(0)
  })

  it('淘汰暂停的旧消息后，新消息仍有完整展示时间', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const toasts = scope.run(() => useToasts())!
    const first = toasts.notify('旧消息')
    toasts.pause(first, 'focus')
    toasts.notify('第二条')
    toasts.notify('第三条')
    vi.advanceTimersByTime(4000)
    toasts.notify('最新消息')
    toasts.resume(first, 'focus')
    vi.advanceTimersByTime(1000)
    expect(toasts.items.value.map(item => item.message)).toEqual(['最新消息'])
    vi.advanceTimersByTime(3999)
    expect(toasts.items.value).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(toasts.items.value).toHaveLength(0)
    scope.stop()
  })

  it('悬停和聚焦分别暂停，离开指针不会提前恢复仍聚焦的消息', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const toasts = scope.run(() => useToasts())!
    const id = toasts.notify('保留输入', 'warning')
    vi.advanceTimersByTime(1000)
    toasts.pause(id, 'pointer')
    toasts.pause(id, 'focus')
    vi.advanceTimersByTime(9000)
    toasts.resume(id, 'pointer')
    vi.advanceTimersByTime(9000)
    expect(toasts.items.value).toHaveLength(1)
    toasts.resume(id, 'focus')
    vi.advanceTimersByTime(3999)
    expect(toasts.items.value).toHaveLength(1)
    vi.advanceTimersByTime(1)
    expect(toasts.items.value).toHaveLength(0)
    scope.stop()
  })

  it('错误保留七秒，普通消息五秒；后台暂停也覆盖新提示', () => {
    vi.useFakeTimers()
    const scope = effectScope()
    const toasts = scope.run(() => useToasts())!
    toasts.notify('保存完成', 'success')
    toasts.notify('保存失败', 'error')
    vi.advanceTimersByTime(5000)
    expect(toasts.items.value.map(item => item.message)).toEqual(['保存失败'])
    toasts.pauseAll('hidden')
    toasts.notify('后台到达', 'info')
    vi.advanceTimersByTime(10000)
    expect(toasts.items.value).toHaveLength(2)
    toasts.resumeAll('hidden')
    vi.advanceTimersByTime(2000)
    expect(toasts.items.value.map(item => item.message)).toEqual(['后台到达'])
    vi.advanceTimersByTime(3000)
    expect(toasts.items.value).toHaveLength(0)
    scope.stop()
  })
})
