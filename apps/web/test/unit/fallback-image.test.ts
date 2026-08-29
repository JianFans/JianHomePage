import { mount } from '@vue/test-utils'
import FallbackImage from '../../components/ui/FallbackImage.vue'

describe('FallbackImage', () => {
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
