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
    const more = wrapper.get('[data-testid="music-more"]')
    expect(more.attributes('aria-label')).toMatch(/查看全部音乐|View all music/)
    expect(more.attributes('href')).toBe(
      'https://y.qq.com/n/ryqq_v2/singer/0036zydh4H05PB',
    )
  })

  it('按后台顺序渲染所有有内容的启用板块', async () => {
    const wrapper = await mountSuspended(HomePage)

    expect(wrapper.findAll('[data-home-section]').map(section => (
      section.attributes('data-home-section')
    ))).toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
  })

  it('在页尾保留品牌域名和官方平台入口', async () => {
    const wrapper = await mountSuspended(HomePage)
    const footer = wrapper.get('footer')

    expect(footer.text()).toContain('yujian.me')
    expect(footer.text()).toContain('© 2026')
    expect(footer.find('[href="https://yujian.me"]').exists()).toBe(true)
    expect(footer.find('[aria-label="微博"], [aria-label="Weibo"]').exists()).toBe(true)
  })
})
