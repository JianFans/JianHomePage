import { mountSuspended } from '@nuxt/test-utils/runtime'
import HomePage from '../../pages/index.vue'

describe('公开首页', () => {
  it('静态渲染品牌、音乐人和首屏媒体', async () => {
    const wrapper = await mountSuspended(HomePage)

    expect(['遇健我', 'Meet Jian']).toContain(wrapper.get('h1').text())
    expect(wrapper.text()).toMatch(/王子健|Wang Zijian/)
    expect(wrapper.get('[data-testid="hero-media"] img').attributes('src')).toBe(
      '/media/hero-studio.webp',
    )
  })

  it('按首页配置渲染最新音乐作品', async () => {
    const wrapper = await mountSuspended(HomePage)

    expect(wrapper.find('#music').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="music-card"]')).toHaveLength(5)
  })
})
