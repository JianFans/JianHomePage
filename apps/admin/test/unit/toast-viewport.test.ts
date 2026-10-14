import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ToastViewport from '../../components/ToastViewport.vue'

describe('Toast viewport', () => {
  it('renders untrusted text safely and exposes independent close and pause events', async () => {
    const wrapper = mount(ToastViewport, { props: {
      items: [{ id: 1, message: '<img src=x onerror=alert(1)>', tone: 'error' }], locale: 'en',
    } })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toBe('<img src=x onerror=alert(1)>')
    await wrapper.get('article').trigger('pointerenter')
    await wrapper.get('article').trigger('focusin')
    await wrapper.get('article').trigger('pointerleave')
    expect(wrapper.emitted('pause')).toEqual([[1, 'pointer'], [1, 'focus']])
    expect(wrapper.emitted('resume')).toEqual([[1, 'pointer']])
    await wrapper.get('button[aria-label="Dismiss message"]').trigger('click')
    expect(wrapper.emitted('dismiss')).toEqual([[1]])
    wrapper.unmount()
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
