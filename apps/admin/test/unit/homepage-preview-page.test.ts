import type { YujianContentSnapshot } from '@yujian/schema'
import { mountSuspended } from '@nuxt/test-utils/runtime'
import { flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import PreviewPage from '../../pages/preview.vue'
import { createLocaleMessage, createSnapshotMessage } from '../../utils/homepage-preview'

const snapshot = fixture as unknown as YujianContentSnapshot
let parentDescriptor: PropertyDescriptor | undefined
const parentWindow = { postMessage: vi.fn() } as unknown as Window
const media: TestAudio[] = []

/** Replace only browser playback; the actual isolated audio controller owns all listeners/state. */
class TestAudio extends EventTarget {
  src = ''
  preload = ''
  currentTime = 0
  duration = 30
  paused = true
  play = vi.fn(async () => { this.paused = false })
  pause = vi.fn(() => { this.paused = true })
  load = vi.fn()
  constructor() { super(); media.push(this) }
}

/** Deliver parent messages without mocking the canonical validator or receiver. */
function send(data: unknown, source: Window = parentWindow, origin = location.origin) {
  window.dispatchEvent(new MessageEvent('message', { data, source, origin }))
}

beforeEach(() => {
  parentDescriptor = Object.getOwnPropertyDescriptor(window, 'parent')
  Object.defineProperty(window, 'parent', { configurable: true, value: parentWindow })
  vi.stubGlobal('Audio', TestAudio)
  media.length = 0
})
afterEach(() => {
  if (parentDescriptor) Object.defineProperty(window, 'parent', parentDescriptor)
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('首页预览接收页面', () => {
  it('挂好监听后发送 ready，直接打开及非法事件保持空态，不请求内容 API', async () => {
    const fetcher = vi.spyOn(globalThis, 'fetch')
    const wrapper = await mountSuspended(PreviewPage)
    expect(parentWindow.postMessage).toHaveBeenCalledWith({ channel: 'yujian-homepage-preview-v1', type: 'ready' }, location.origin)
    expect(wrapper.find('h1').exists()).toBe(false)
    const message = createSnapshotMessage({ snapshot, locale: 'en' })
    send(message, window)
    send(message, parentWindow, 'https://wrong.example.com')
    send({ ...message, locale: 'fr' })
    send({ ...message, snapshot: {} })
    send({ ...message, channel: 'other' })
    await nextTick()
    expect(wrapper.find('h1').exists()).toBe(false)
    expect(fetcher).not.toHaveBeenCalled()
    wrapper.unmount()
    Object.defineProperty(window, 'parent', { configurable: true, value: window })
    const direct = await mountSuspended(PreviewPage)
    send(message, window)
    await nextTick()
    expect(direct.find('h1').exists()).toBe(false)
    direct.unmount()
  })

  it('渲染真实首页、独立切换语言并拦截外链及辅助点击，保留页面锚点', async () => {
    const wrapper = await mountSuspended(PreviewPage)
    send(createSnapshotMessage({ snapshot, locale: 'en' }))
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('Meet Jian')
    expect(wrapper.findAll('#top')).toHaveLength(1)
    const external = wrapper.get('a[href^="https:"]').element
    for (const type of ['click', 'auxclick']) {
      const click = new MouseEvent(type, { bubbles: true, cancelable: true, button: type === 'auxclick' ? 1 : 0 })
      external.dispatchEvent(click)
      expect(click.defaultPrevented).toBe(true)
    }
    const anchor = new MouseEvent('click', { bubbles: true, cancelable: true })
    wrapper.get('a[href="#music"]').element.dispatchEvent(anchor)
    expect(anchor.defaultPrevented).toBe(false)
    await wrapper.get('[aria-label="切换为中文"]').trigger('click')
    expect(wrapper.get('h1').text()).toBe('遇健我')
    expect(parentWindow.postMessage).toHaveBeenLastCalledWith(createLocaleMessage('zh-CN'), location.origin)
    send(createLocaleMessage('en'))
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('Meet Jian')
    wrapper.unmount()
  })

  it('快照替换重挂展示并释放音频，pagehide 与卸载清理媒体和监听', async () => {
    const remove = vi.spyOn(window, 'removeEventListener')
    const wrapper = await mountSuspended(PreviewPage)
    send(createSnapshotMessage({ snapshot, locale: 'en' }))
    await nextTick()
    const oldHero = wrapper.get('[data-home-section="hero"]').element
    const preview = wrapper.findAll('button').find(button => button.attributes('aria-label')?.startsWith('Preview '))!
    await preview.trigger('click')
    await flushPromises()
    expect(media).toHaveLength(1)
    expect(wrapper.find('[data-testid="audio-dock"]').exists()).toBe(true)
    expect(wrapper.classes()).toContain('preview--audio-active')
    const oldMedia = media[0]!
    const removeMedia = vi.spyOn(oldMedia, 'removeEventListener')
    const replacement = structuredClone(snapshot)
    replacement.site.brand.en = 'Updated draft'
    send(createSnapshotMessage({ snapshot: replacement, locale: 'en' }))
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('Updated draft')
    expect(wrapper.get('[data-home-section="hero"]').element).not.toBe(oldHero)
    expect(oldMedia.src).toBe('')
    expect(oldMedia.pause).toHaveBeenCalled()
    expect(removeMedia).toHaveBeenCalledTimes(4)
    oldMedia.dispatchEvent(new Event('error'))
    expect(wrapper.find('[data-testid="audio-dock"]').exists()).toBe(false)
    const nextPreview = wrapper.findAll('button').find(button => button.attributes('aria-label')?.startsWith('Preview '))!
    await nextPreview.trigger('click')
    await flushPromises()
    window.dispatchEvent(new Event('pagehide'))
    await nextTick()
    expect(media[1]!.src).toBe('')
    expect(wrapper.find('[data-home-section="hero"]').exists()).toBe(false)
    parentWindow.postMessage = vi.fn()
    const restored = new Event('pageshow')
    Object.defineProperty(restored, 'persisted', { value: true })
    window.dispatchEvent(restored)
    expect(parentWindow.postMessage).toHaveBeenCalledWith({ channel: 'yujian-homepage-preview-v1', type: 'ready' }, location.origin)
    send(createSnapshotMessage({ snapshot, locale: 'en' }))
    await nextTick()
    expect(wrapper.get('h1').text()).toBe('Meet Jian')
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('message', expect.any(Function))
    expect(remove).toHaveBeenCalledWith('pagehide', expect.any(Function))
  })

  it('视频默认不自动播放，保留手动播放并在快照替换时释放旧视频', async () => {
    const videoSnapshot = structuredClone(snapshot)
    const slide = videoSnapshot.heroSlides[0]!
    const image = videoSnapshot.assets.find(asset => asset.id === slide.assetId)!
    videoSnapshot.assets.push({ ...image, id: 'asset_preview_video', kind: 'video', src: '/media/preview.mp4', mimeType: 'video/mp4' })
    slide.mediaKind = 'video'
    slide.assetId = 'asset_preview_video'
    slide.autoplay = true
    const wrapper = await mountSuspended(PreviewPage)
    send(createSnapshotMessage({ snapshot: videoSnapshot, locale: 'en' }))
    await nextTick()
    const video = wrapper.get('video').element
    expect(video.autoplay).toBe(false)
    const play = vi.spyOn(video, 'play').mockImplementation(async () => {})
    const pause = vi.spyOn(video, 'pause')
    const load = vi.spyOn(video, 'load')
    await wrapper.get('[aria-label="Play video"]').trigger('click')
    expect(play).toHaveBeenCalledOnce()
    send(createSnapshotMessage({ snapshot, locale: 'en' }))
    await nextTick()
    expect(pause).toHaveBeenCalled()
    expect(load).toHaveBeenCalled()
    expect(video.getAttribute('src')).toBeNull()
    wrapper.unmount()
  })
})
