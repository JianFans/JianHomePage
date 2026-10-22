import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import FallbackImage from '../../components/ui/FallbackImage.vue'

describe('FallbackImage', () => {
  it.each([720, 0])('换源后已完成的缓存图片无需事件即可结束等待，naturalWidth=%s', async (naturalWidth) => {
    const complete = vi.spyOn(HTMLImageElement.prototype, 'complete', 'get').mockReturnValue(false)
    const width = vi.spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get').mockReturnValue(naturalWidth)
    const wrapper = mount(FallbackImage, { props: { src: '/media/first.webp', alt: '封面' } })
    try {
      complete.mockReturnValue(true)
      await wrapper.setProps({ src: '/media/cached-next.webp' })
      await flushPromises()
      expect(wrapper.get('img').attributes('data-media-state')).toBe(naturalWidth > 0 ? 'ready' : 'error')
      if (naturalWidth > 0) expect(wrapper.get('img').attributes('src')).toBe('/media/cached-next.webp')
      else expect(wrapper.get('img').attributes('src')).toMatch(/^data:image\/svg\+xml/)
    } finally { wrapper.unmount(); complete.mockRestore(); width.mockRestore() }
  })

  it('暴露真实加载状态，换源后拒绝旧图片的迟到事件', async () => {
    const wrapper = mount(FallbackImage, { props: { src: '/media/first.webp', alt: '封面', width: 720, height: 720 } })
    const old = wrapper.get('img').element
    expect(wrapper.get('img').attributes('data-media-state')).toBe('loading')
    await wrapper.get('img').trigger('load')
    expect(wrapper.get('img').attributes('data-media-state')).toBe('ready')
    await wrapper.setProps({ src: '/media/next.webp' })
    expect(wrapper.get('img').attributes('data-media-state')).toBe('loading')
    old.dispatchEvent(new Event('load'))
    old.dispatchEvent(new Event('error'))
    expect(wrapper.get('img').attributes('data-media-state')).toBe('loading')
    expect(wrapper.get('img').attributes('src')).toBe('/media/next.webp')
    await wrapper.get('img').trigger('error')
    expect(wrapper.get('img').attributes('data-media-state')).toBe('error')
    wrapper.unmount()
  })

  it('缓存命中无需额外 load 事件即可结束等待', async () => {
    const complete = vi.spyOn(HTMLImageElement.prototype, 'complete', 'get').mockReturnValue(true)
    const width = vi.spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get').mockReturnValue(720)
    try {
      const wrapper = mount(FallbackImage, { props: { src: '/media/cached.webp', alt: '缓存图片' } })
      await nextTick()
      expect(wrapper.get('img').attributes('data-media-state')).toBe('ready')
      wrapper.unmount()
    } finally { complete.mockRestore(); width.mockRestore() }
  })

  it('挂载前图片已失败时不依赖错过的 error 事件，结束等待并显示占位', async () => {
    const complete = vi.spyOn(HTMLImageElement.prototype, 'complete', 'get').mockReturnValue(true)
    const width = vi.spyOn(HTMLImageElement.prototype, 'naturalWidth', 'get').mockReturnValue(0)
    try {
      const wrapper = mount(FallbackImage, { props: { src: '/media/failed-before-hydration.webp', alt: '缺失封面' } })
      await nextTick()
      expect(wrapper.get('img').attributes('data-media-state')).toBe('error')
      expect(wrapper.get('img').attributes('src')).toMatch(/^data:image\/svg\+xml/)
      wrapper.unmount()
    } finally { complete.mockRestore(); width.mockRestore() }
  })
  it('图片失败时保留原尺寸和替代文本并切换到内嵌占位图', async () => {
    const wrapper = mount(FallbackImage, {
      props: {
        src: '/media/missing.webp',
        alt: '缺失封面',
        width: 720,
        height: 720,
      },
    })
    const image = wrapper.get('img')

    await image.trigger('error')

    expect(image.attributes('src')).toMatch(/^data:image\/svg\+xml/)
    expect(image.attributes()).toMatchObject({
      alt: '缺失封面',
      width: '720',
      height: '720',
    })
    expect(image.attributes('data-fallback')).toBe('true')
  })
})
