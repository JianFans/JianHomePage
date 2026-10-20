import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, reactive } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import ToastViewport from '../../components/ToastViewport.vue'
import { useToasts } from '../../composables/useToasts'

afterEach(() => { vi.useRealTimers() })

describe('Toast viewport', () => {
  it('renders untrusted text safely and exposes independent close and pause events', async () => {
    const wrapper = mount(ToastViewport, { props: {
      items: [{ id: 'toast_550e8400-e29b-41d4-a716-446655440000', message: '<img src=x onerror=alert(1)>', tone: 'error' }], locale: 'en',
    } })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toBe('<img src=x onerror=alert(1)>')
    await wrapper.get('article').trigger('pointerenter')
    await wrapper.get('article').trigger('focusin')
    await wrapper.get('article').trigger('pointerleave')
    const id = 'toast_550e8400-e29b-41d4-a716-446655440000'
    expect(wrapper.emitted('pause')).toEqual([[id, 'pointer']])
    expect(wrapper.emitted('pauseAll')).toEqual([['focus']])
    expect(wrapper.emitted('resume')).toEqual([[id, 'pointer']])
    await wrapper.get('button[aria-label="Dismiss message"]').trigger('click')
    expect(wrapper.emitted('dismiss')).toEqual([[id]])
    wrapper.unmount()
  })

  it('焦点在视口及关闭按钮之间移动时暂停全部消息，离开后恢复剩余时间', async () => {
    vi.useFakeTimers()
    const host = defineComponent({
      components: { ToastViewport },
      /** 使用真实计时器状态验证视口的全局焦点事件。 */
      setup() { return { toasts: reactive(useToasts()) } },
      template: '<ToastViewport :items="toasts.items" locale="en" @pause-all="toasts.pauseAll" @resume-all="toasts.resumeAll" />',
    })
    const wrapper = mount(host, { attachTo: document.body })
    const outside = document.createElement('button')
    document.body.append(outside)
    try {
      const toasts = wrapper.vm.toasts
      toasts.notify('First')
      toasts.notify('Second')
      await nextTick()
      vi.advanceTimersByTime(1000)
      const viewport = wrapper.get('[data-testid="toast-viewport"]')
      ;(viewport.element as HTMLElement).focus()
      vi.advanceTimersByTime(9000)
      expect(toasts.items).toHaveLength(2)
      toasts.notify('Arrived while focused')
      await nextTick()
      const buttons = wrapper.findAll('button')
      ;(buttons[0]!.element as HTMLElement).focus()
      vi.advanceTimersByTime(9000)
      ;(buttons[1]!.element as HTMLElement).focus()
      vi.advanceTimersByTime(9000)
      expect(toasts.items).toHaveLength(3)
      outside.focus()
      vi.advanceTimersByTime(3999)
      expect(toasts.items).toHaveLength(3)
      vi.advanceTimersByTime(1)
      expect(toasts.items.map(item => item.message)).toEqual(['Arrived while focused'])
      vi.advanceTimersByTime(1000)
      expect(toasts.items).toHaveLength(0)
      toasts.notify('Focused old message')
      await nextTick()
      ;(wrapper.get('button').element as HTMLElement).focus()
      for (const message of ['Replacement 1', 'Replacement 2', 'Replacement 3']) toasts.notify(message)
      await nextTick()
      vi.advanceTimersByTime(5000)
      expect(toasts.items).toHaveLength(0)
    } finally { wrapper.unmount(); outside.remove() }
  })

  it('离场结束后复查焦点，无 focusout 时释放暂停，焦点仍在区域内时保持暂停', () => {
    const wrapper = mount(ToastViewport, { attachTo: document.body, props: {
      items: [{ id: 'toast_550e8400-e29b-41d4-a716-446655440000', message: 'Closing', tone: 'info' }], locale: 'en',
    } })
    try {
      const button = wrapper.get('button').element as HTMLElement
      button.focus()
      const transition = wrapper.findComponent({ name: 'TransitionGroup' })
      transition.vm.$emit('afterLeave')
      expect(wrapper.emitted('resumeAll')).toBeUndefined()
      // 模拟节点移除未向视口冒泡 focusout 的浏览器行为。
      button.addEventListener('focusout', event => event.stopPropagation(), { once: true })
      button.remove()
      expect(wrapper.element.contains(document.activeElement)).toBe(false)
      expect(wrapper.emitted('resumeAll')).toBeUndefined()
      transition.vm.$emit('afterLeave')
      expect(wrapper.emitted('resumeAll')).toEqual([['focus']])
    } finally { wrapper.unmount() }
  })

  it('reports page visibility and removes its listener when unmounted', async () => {
    const spy = vi.spyOn(document, 'removeEventListener')
    const wrapper = mount(ToastViewport, { props: { items: [], locale: 'zh-CN' } })
    document.dispatchEvent(new Event('visibilitychange'))
    expect(wrapper.emitted('visibility')?.at(-1)).toEqual([document.hidden])
    wrapper.unmount()
    expect(spy).toHaveBeenCalledWith('visibilitychange', expect.any(Function))
    spy.mockRestore()
  })
})
