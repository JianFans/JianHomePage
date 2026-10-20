import type { Asset, HeroSlide } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import HeroShowcase from '../../components/home/HeroShowcase.vue'

const imageAssets = [
  {
    id: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
    kind: 'image',
    src: '/media/hero-studio.webp',
    mimeType: 'image/webp',
    byteSize: 1,
    width: 1920,
    height: 1200,
    alt: { 'zh-CN': '录音室', en: 'Studio' },
    rights: { source: { 'zh-CN': '测试' } },
    checksum: `sha256:${'0'.repeat(64)}`,
  },
  {
    id: 'asset_23f413dc-7217-4cbf-bf96-e7c7a740abb4',
    kind: 'image',
    src: '/media/hero-stage.webp',
    mimeType: 'image/webp',
    byteSize: 1,
    width: 1920,
    height: 1200,
    alt: { 'zh-CN': '舞台', en: 'Stage' },
    rights: { source: { 'zh-CN': '测试' } },
    checksum: `sha256:${'1'.repeat(64)}`,
  },
] as Asset[]

const imageSlides = [
  {
    id: 'hero_340f8cbb-68b0-4a31-a624-ab18651a58ad',
    mediaKind: 'image',
    assetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
    focalPoint: { x: 34, y: 58 },
    headline: { 'zh-CN': '遇健我', en: 'Meet Jian' },
    autoplay: false,
  },
  {
    id: 'hero_1a1d20aa-ad46-4c41-8cb6-d576b334cecb',
    mediaKind: 'image',
    assetId: 'asset_23f413dc-7217-4cbf-bf96-e7c7a740abb4',
    focalPoint: { x: 62, y: 40 },
    headline: { 'zh-CN': '示例现场', en: 'Sample Live' },
    autoplay: false,
  },
] as HeroSlide[]

function stubReducedMotion(matches: boolean) {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
    matches,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('HeroShowcase', () => {
  it('跳转独占翻页上方的稳定槽位，无目标时仍保留翻页结构', async () => {
    stubReducedMotion(false)
    const slides = structuredClone(imageSlides)
    slides[0]!.target = { kind: 'external', link: { provider: 'other', url: 'https://yujian.me' } }
    const wrapper = mount(HeroShowcase, { props: { slides, assets: imageAssets, locale: 'en', brand: 'Meet Jian', artistName: 'Wang Zijian' } })
    expect(wrapper.get('.hero-navigation > .hero-link-slot > a').attributes('href')).toBe('https://yujian.me')
    expect(wrapper.findAll('.hero-pagination button')).toHaveLength(2)
    await wrapper.get('[data-testid="hero-next"]').trigger('click')
    expect(wrapper.find('.hero-link-slot').exists()).toBe(true)
    expect(wrapper.find('.hero-link-slot a').exists()).toBe(false)
    expect(wrapper.findAll('.hero-pagination button')).toHaveLength(2)
    wrapper.unmount()
  })
  it('渲染图片、焦点坐标和稳定页进度', async () => {
    stubReducedMotion(false)
    const wrapper = mount(HeroShowcase, {
      props: {
        slides: imageSlides,
        assets: imageAssets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    expect(wrapper.get('img').attributes('src')).toBe('/media/hero-studio.webp')
    expect(wrapper.get('img').attributes('alt')).toBe('录音室')
    expect(wrapper.get('[data-testid="hero-media"]').attributes('style')).toContain('--focal-x: 34%')
    expect(wrapper.findAll('[data-hero-progress]')).toHaveLength(2)
    expect(wrapper.findAll('[data-hero-progress]')[0]!.attributes('aria-current')).toBe('true')

    await wrapper.get('[data-testid="hero-next"]').trigger('click')
    expect(wrapper.get('img').attributes('src')).toBe('/media/hero-stage.webp')
    expect(wrapper.findAll('[data-hero-progress]')[1]!.attributes('aria-current')).toBe('true')
  })

  it('为每个 Hero 内容 ID 提供稳定锚点', () => {
    stubReducedMotion(false)
    const wrapper = mount(HeroShowcase, {
      props: {
        slides: imageSlides,
        assets: imageAssets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    expect(wrapper.findAll('[data-content-anchor]').map(anchor => anchor.attributes('id')))
      .toEqual(['hero_340f8cbb-68b0-4a31-a624-ab18651a58ad', 'hero_1a1d20aa-ad46-4c41-8cb6-d576b334cecb'])
  })

  it('为配置的移动媒体输出响应式 source', () => {
    stubReducedMotion(false)
    const assets = [
      ...imageAssets,
      {
        ...imageAssets[0],
        id: 'asset_b256524c-9e1c-4f02-a10c-71f01ed6de85',
        src: '/media/hero-mobile.webp',
        width: 900,
        height: 1200,
      },
    ] as Asset[]
    const slides = [{
      ...imageSlides[0],
      mobileAssetId: 'asset_b256524c-9e1c-4f02-a10c-71f01ed6de85',
    }] as HeroSlide[]

    const wrapper = mount(HeroShowcase, {
      props: {
        slides,
        assets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    expect(wrapper.get('picture source').attributes('media')).toBe('(max-width: 42rem)')
    expect(wrapper.get('picture source').attributes('srcset')).toBe('/media/hero-mobile.webp')
    expect(wrapper.get('picture img').attributes('src')).toBe('/media/hero-studio.webp')
  })

  it('视频提供静音和暂停控制', () => {
    stubReducedMotion(false)
    const assets = [
      ...imageAssets,
      {
        ...imageAssets[0],
        id: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
        kind: 'video',
        src: '/media/hero.mp4',
        mimeType: 'video/mp4',
        posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
        durationSeconds: 12,
      },
    ] as Asset[]
    const slides = [{
      ...imageSlides[0],
      id: 'hero_b55bc8ea-1ea6-4f4c-9297-dcdf679a986c',
      mediaKind: 'video',
      assetId: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
      posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
      autoplay: true,
    }] as HeroSlide[]

    const wrapper = mount(HeroShowcase, {
      props: {
        slides,
        assets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    expect(wrapper.get('video').attributes('muted')).toBeDefined()
    expect(wrapper.get('video').attributes('playsinline')).toBeDefined()
    expect(wrapper.get('[aria-label="暂停视频"]').attributes('type')).toBe('button')
    expect(wrapper.get('[aria-label="开启声音"]').attributes('type')).toBe('button')
  })

  it('减少动态效果时不加载自动视频并显示海报', () => {
    stubReducedMotion(true)
    const assets = [
      ...imageAssets,
      {
        ...imageAssets[0],
        id: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
        kind: 'video',
        src: '/media/hero.mp4',
        mimeType: 'video/mp4',
        posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
        durationSeconds: 12,
      },
    ] as Asset[]
    const slides = [{
      ...imageSlides[0],
      id: 'hero_b55bc8ea-1ea6-4f4c-9297-dcdf679a986c',
      mediaKind: 'video',
      assetId: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
      posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
      autoplay: true,
    }] as HeroSlide[]

    const wrapper = mount(HeroShowcase, {
      props: {
        slides,
        assets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('/media/hero-studio.webp')
  })

  it('视频加载失败时回退海报并隐藏失效控制', async () => {
    stubReducedMotion(false)
    const assets = [
      ...imageAssets,
      {
        ...imageAssets[0],
        id: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
        kind: 'video',
        src: '/media/hero.mp4',
        mimeType: 'video/mp4',
        posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
        durationSeconds: 12,
      },
    ] as Asset[]
    const slides = [{
      ...imageSlides[0],
      id: 'hero_b55bc8ea-1ea6-4f4c-9297-dcdf679a986c',
      mediaKind: 'video',
      assetId: 'asset_2ee541f7-519a-4564-afd2-758d17f9263d',
      posterAssetId: 'asset_799acd0f-add2-4915-bca7-688cbcf07c63',
      autoplay: true,
    }] as HeroSlide[]
    const wrapper = mount(HeroShowcase, {
      props: {
        slides,
        assets,
        locale: 'zh-CN',
        brand: '遇健我',
        artistName: '王子健',
      },
    })

    await wrapper.get('video').trigger('error')

    expect(wrapper.find('video').exists()).toBe(false)
    expect(wrapper.get('img').attributes('src')).toBe('/media/hero-studio.webp')
    expect(wrapper.find('[aria-label="暂停视频"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="开启声音"]').exists()).toBe(false)
  })
})
