import type { YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import HomepagePreview from '../../components/HomepagePreview.vue'
import { createLocaleMessage, createReadyMessage } from '../../utils/homepage-preview'

const snapshot = fixture as unknown as YujianContentSnapshot

/** Detached happy-dom iframes have no Window; replace only that unavailable browser boundary. */
function frameMessages(wrapper: ReturnType<typeof mount>) {
  const frame = wrapper.get('iframe').element as HTMLIFrameElement
  const post = vi.fn()
  const source = { postMessage: post } as unknown as Window
  Object.defineProperty(frame, 'contentWindow', { configurable: true, value: source })
  return { frame, source, post }
}

/** Dispatch a browser-shaped handshake, allowing tests to challenge either identity check. */
function ready(source: Window, origin = location.origin, data: unknown = createReadyMessage()) {
  window.dispatchEvent(new MessageEvent('message', { data, origin, source }))
}

afterEach(() => { vi.restoreAllMocks(); vi.useRealTimers(); vi.unstubAllGlobals() })

describe('隔离首页预览宿主', () => {
  it('默认关闭，仅当前 iframe 握手后发送快照，load 不撤销较早 ready', async () => {
    const wrapper = mount(HomepagePreview, { props: { snapshot, textIsCurrent: true, textRevision: 0, formDirty: false, locale: 'zh-CN' } })
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    const { frame, source, post } = frameMessages(wrapper)
    expect(wrapper.get('.homepage-preview__stage').attributes('aria-busy')).toBe('true')
    expect(wrapper.get('.homepage-preview__viewport [data-testid="preview-waiting"]').text()).toContain('预览')
    expect(frame.hasAttribute('inert')).toBe(true)
    expect(frame.getAttribute('src')).toBe('/preview')
    expect(frame.getAttribute('sandbox')).toBe('allow-scripts allow-same-origin')
    expect(frame.getAttribute('referrerpolicy')).toBe('no-referrer')
    ready(source, 'https://wrong.example.com')
    ready(window)
    ready(source, location.origin, { ...createReadyMessage(), channel: 'wrong' })
    expect(post).not.toHaveBeenCalled()
    ready(source)
    await nextTick()
    expect(wrapper.get('.homepage-preview__stage').attributes('aria-busy')).toBe('false')
    expect(frame.hasAttribute('inert')).toBe(false)
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ type: 'snapshot', locale: 'zh-CN', snapshot }), location.origin)
    await wrapper.get('iframe').trigger('load')
    expect(wrapper.find('[data-testid="preview-waiting"]').exists()).toBe(false)
    await wrapper.get('[data-testid="preview-language"]').trigger('click')
    expect(post).toHaveBeenLastCalledWith(createLocaleMessage('en'), location.origin)
    expect(localStorage.getItem('yujian-preview-locale')).toBeNull()
    wrapper.unmount()
  })

  it('文本过期立即撤下 iframe，恢复原文也创建新文档，并提示未应用表单', async () => {
    const wrapper = mount(HomepagePreview, { props: { snapshot, textIsCurrent: true, textRevision: 0, formDirty: true, locale: 'en' } })
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    const oldFrame = wrapper.get('iframe').element
    expect(wrapper.get('[data-testid="preview-form-dirty"]').text()).toContain('applied')
    await wrapper.setProps({ textIsCurrent: false, textRevision: 1 })
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.setProps({ textIsCurrent: true, textRevision: 2 })
    expect(wrapper.get('iframe').element).not.toBe(oldFrame)
    const restored = wrapper.get('iframe').element
    await wrapper.setProps({ textRevision: 4 })
    expect(wrapper.get('iframe').element).not.toBe(restored)
    await wrapper.get('[data-testid="preview-mobile"]').trigger('click')
    expect(wrapper.get('iframe').attributes('width')).toBe('390')
    expect(wrapper.get('iframe').attributes('height')).toBe('780')
    await wrapper.get('[data-testid="preview-desktop"]').trigger('click')
    expect(wrapper.get('iframe').attributes('width')).toBe('1280')
    wrapper.unmount()
  })

  it('load 早于 ready 仍保持等待，子页语言只接受当前文档且不回传循环', async () => {
    const wrapper = mount(HomepagePreview, { props: { snapshot, textIsCurrent: true, textRevision: 0, formDirty: false, locale: 'en' } })
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    const { source, post } = frameMessages(wrapper)
    await wrapper.get('iframe').trigger('load')
    expect(wrapper.find('[data-testid="preview-waiting"]').exists()).toBe(true)
    ready(source)
    await nextTick()
    expect(post).toHaveBeenCalledTimes(1)
    window.dispatchEvent(new MessageEvent('message', { data: createLocaleMessage('zh-CN'), source: window, origin: location.origin }))
    await nextTick()
    expect(wrapper.get('[data-testid="preview-language"]').text()).toBe('EN')
    window.dispatchEvent(new MessageEvent('message', { data: createLocaleMessage('zh-CN'), source, origin: location.origin }))
    await nextTick()
    expect(wrapper.get('[data-testid="preview-language"]').text()).toBe('中')
    expect(post).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('超时后卸载并支持重试，关闭和卸载清理监听、计时器和观察器', async () => {
    vi.useFakeTimers()
    const disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class { observe() {} disconnect = disconnect })
    const remove = vi.spyOn(window, 'removeEventListener')
    const wrapper = mount(HomepagePreview, { props: { snapshot, textIsCurrent: true, textRevision: 0, formDirty: false, locale: 'en' } })
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    const oldFrame = wrapper.get('iframe').element
    await vi.advanceTimersByTimeAsync(10000)
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.get('[data-testid="preview-retry"]').trigger('click')
    expect(wrapper.get('iframe').element).not.toBe(oldFrame)
    await wrapper.get('[data-testid="preview-close"]').trigger('click')
    expect(wrapper.find('iframe').exists()).toBe(false)
    expect(vi.getTimerCount()).toBe(0)
    expect(disconnect).toHaveBeenCalled()
    expect(remove).toHaveBeenCalledWith('message', expect.any(Function))
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('message', expect.any(Function))
  })

  it('通过缩放适配容器，保留 iframe 的真实视口尺寸', async () => {
    let resized!: ResizeObserverCallback
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: ResizeObserverCallback) { resized = callback }
      observe() {}
      disconnect() {}
    })
    const wrapper = mount(HomepagePreview, { props: { snapshot, textIsCurrent: true, textRevision: 0, formDirty: false, locale: 'en' } })
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    resized([{ contentRect: { width: 640 } } as ResizeObserverEntry], {} as ResizeObserver)
    await nextTick()
    expect(wrapper.get('iframe').attributes('width')).toBe('1280')
    expect(wrapper.get('iframe').attributes('style')).toContain('scale(0.5)')
    expect(wrapper.get('.homepage-preview__viewport').attributes('style')).toContain('height: 400px')
    await wrapper.get('[data-testid="preview-mobile"]').trigger('click')
    expect(wrapper.get('iframe').attributes('style')).toContain('scale(1)')
    expect(wrapper.get('.homepage-preview__viewport').attributes('style')).toContain('height: 780px')
    wrapper.unmount()
  })
})
