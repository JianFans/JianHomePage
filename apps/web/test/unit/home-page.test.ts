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

  it('保留平台入口并增加前往音乐目录的入口', async () => {
    const wrapper = await mountSuspended(HomePage, { props: { linkMusicCatalog: true } })

    expect(wrapper.find('#music').exists()).toBe(true)
    expect(wrapper.findAll('[data-testid="music-card"]')).toHaveLength(5)
    const catalog = wrapper.get('[data-testid="music-more"]')
    expect(catalog.attributes('aria-label')).toMatch(/查看全部音乐|View all music/)
    expect(catalog.attributes('href')).toBe('/music/')
    const external = wrapper.get('[data-testid="music-external"]')
    expect(external.attributes('aria-label')).toMatch(/外部音乐链接|External music link/)
    expect(external.attributes('href')).toBe(
      'https://y.qq.com/n/ryqq_v2/singer/0036zydh4H05PB',
    )
  })

  it('在影像板块提供全部作品目录入口', async () => {
    const wrapper = await mountSuspended(HomePage, { props: { linkVideoCatalog: true } })
    const catalog = wrapper.get('[data-testid="video-more"]')
    expect(catalog.attributes('href')).toBe('/video/')
    expect(catalog.attributes('aria-label')).toMatch(/查看全部影像|View all videos/)
  })

  it('按后台顺序渲染所有有内容的启用板块', async () => {
    const wrapper = await mountSuspended(HomePage)

    expect(wrapper.findAll('[data-home-section]').map(section => (
      section.attributes('data-home-section')
    ))).toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
  })

  it('在页尾保留品牌域名，平台入口集中在音乐人板块', async () => {
    const wrapper = await mountSuspended(HomePage)
    const footer = wrapper.get('footer')

    expect(footer.text()).toContain('yujian.me')
    expect(footer.text()).toContain('© 2026')
    expect(footer.find('[href="https://yujian.me"]').exists()).toBe(true)
    expect(footer.find('.platform-links').exists()).toBe(false)
    expect(wrapper.findAll('#artist .platform-links')).toHaveLength(1)
    expect(wrapper.findAll('#artist .platform-links__desktop [aria-label="微博 W"], #artist .platform-links__desktop [aria-label="Weibo W"]')).toHaveLength(1)
    wrapper.unmount()
  })
})
