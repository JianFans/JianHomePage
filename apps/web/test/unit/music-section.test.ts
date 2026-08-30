import type { MusicSection as MusicSectionConfig, YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import fixture from '../../../../content/fixtures/homepage.json'
import MusicSection from '../../components/home/MusicSection.vue'
import PlatformLinks from '../../components/ui/PlatformLinks.vue'

const snapshot = fixture as unknown as YujianContentSnapshot
const section = snapshot.homepage.sections.find(
  (item): item is MusicSectionConfig => item.type === 'music',
)!

function mountMusicSection(locale: 'zh-CN' | 'en' = 'zh-CN') {
  return mount(MusicSection, {
    props: {
      section,
      releases: snapshot.releases,
      tracks: snapshot.tracks,
      assets: snapshot.assets,
      locale,
      activeTrackId: null,
      playerStatus: 'idle',
    },
  })
}

describe('MusicSection', () => {
  it('按配置顺序显示稳定方形封面的作品卡', () => {
    const wrapper = mountMusicSection()
    const cards = wrapper.findAll('[data-testid="music-card"]')

    expect(cards).toHaveLength(5)
    expect(cards[0]!.classes()).toContain('music-card--featured')
    expect(cards[0]!.get('.music-cover').classes()).toContain('music-cover')
    expect(cards[0]!.text()).toContain('示例作品 01')
  })

  it('为作品和曲目内容 ID 提供稳定锚点', () => {
    const wrapper = mountMusicSection()

    expect(wrapper.find('#release_01').exists()).toBe(true)
    expect(wrapper.find('#track_01').exists()).toBe(true)
    expect(wrapper.find('#track_05').exists()).toBe(true)
  })

  it('只有具备试听资源的作品才在封面显示播放按钮', async () => {
    const wrapper = mountMusicSection()
    const cards = wrapper.findAll('[data-testid="music-card"]')

    expect(cards[0]!.find('[aria-label="试听示例曲目 01"]').exists()).toBe(true)
    expect(cards[1]!.find('[data-testid="preview-trigger"]').exists()).toBe(false)

    await cards[0]!.get('[data-testid="preview-trigger"]').trigger('click')
    expect(wrapper.emitted('preview')?.[0]?.[0]).toMatchObject({
      id: 'track_01',
      previewSrc: '/media/preview-sample.wav',
    })
  })

  it('英文试听按钮名称使用可读的单词间隔', () => {
    const wrapper = mountMusicSection('en')

    expect(wrapper.get('[data-testid="preview-trigger"]').attributes('aria-label'))
      .toBe('Preview Sample Track 01')
  })

  it('将平台入口放在卡片操作区并使用安全外链属性', () => {
    const wrapper = mountMusicSection()
    const links = wrapper.findAll('[data-testid="music-card"]')[0]!
      .get('[data-testid="platform-links"]')
      .findAll('a')

    expect(links.length).toBeGreaterThan(1)
    expect(links.every(link => link.attributes('target') === '_blank')).toBe(true)
    expect(links.every(link => link.attributes('rel') === 'noopener noreferrer')).toBe(true)
  })

  it('提供窄屏主平台与更多平台的渐进结构', () => {
    const wrapper = mountMusicSection()
    const platforms = wrapper.findAll('[data-testid="platform-links"]')[0]!

    expect(platforms.find('[data-testid="platform-primary"]').exists()).toBe(true)
    expect(platforms.find('[data-testid="platform-more"]').exists()).toBe(true)
  })
})

describe('PlatformLinks', () => {
  it('允许靠左的平台组让移动菜单向右展开', () => {
    const wrapper = mount(PlatformLinks, {
      props: {
        locale: 'zh-CN',
        menuAlign: 'start',
        links: [
          { provider: 'weibo', url: 'https://weibo.com/example' },
          { provider: 'bilibili', url: 'https://space.bilibili.com/example' },
        ],
      },
    })

    expect(wrapper.get('[data-testid="platform-more"]').classes()).toContain('platform-more--start')
  })

  it('忽略非 HTTPS 平台链接', () => {
    const wrapper = mount(PlatformLinks, {
      props: {
        locale: 'zh-CN',
        links: [
          { provider: 'qq-music', url: 'https://y.qq.com/safe' },
          { provider: 'other', url: 'http://example.com/unsafe' },
        ],
      },
    })

    expect(wrapper.findAll('a')).toHaveLength(2)
    expect(wrapper.findAll('a').every(link => link.attributes('href') === 'https://y.qq.com/safe')).toBe(true)
  })
})
