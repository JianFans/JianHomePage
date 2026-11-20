import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json'
import { internalContentUrl, momentCatalogMetadata, momentCatalogUrl } from '../../utils/moments-catalog'

const snapshot = fixture as unknown as YujianContentSnapshot

describe('片段目录', () => {
  it('按站点 canonical 生成稳定 URL 和双语元数据', () => {
    expect(momentCatalogUrl('https://release.yujian.me')).toBe('https://release.yujian.me/moments/')
    expect(momentCatalogMetadata(snapshot, 'zh-CN')).toMatchObject({
      title: expect.stringContaining('全部片段'),
      canonical: 'https://yujian.me/moments/',
    })
    expect(momentCatalogMetadata(snapshot, 'en').title).toContain('Moments')
  })

  it('将内部内容目标路由到匹配类型的目录锚点', () => {
    expect(internalContentUrl(snapshot, snapshot.artist.id)).toBe(`/#${snapshot.artist.id}`)
    expect(internalContentUrl(snapshot, snapshot.heroSlides[0]!.id)).toBe(`/#${snapshot.heroSlides[0]!.id}`)
    expect(internalContentUrl(snapshot, snapshot.releases[0]!.id)).toBe(`/music/#${snapshot.releases[0]!.id}`)
    expect(internalContentUrl(snapshot, snapshot.tracks[0]!.id)).toBe(`/music/#${snapshot.tracks[0]!.id}`)
    expect(internalContentUrl(snapshot, snapshot.videos[0]!.id)).toBe(`/video/#${snapshot.videos[0]!.id}`)
    expect(internalContentUrl(snapshot, snapshot.events[0]!.id)).toBe(`/events/#${snapshot.events[0]!.id}`)
    expect(internalContentUrl(snapshot, snapshot.moments[0]!.id)).toBe(`/moments/#${snapshot.moments[0]!.id}`)
    expect(internalContentUrl(snapshot, 'missing')).toBeNull()
  })

  it('不为首页中未渲染的艺人和轮播锚点生成链接', () => {
    const hiddenArtist = structuredClone(snapshot)
    hiddenArtist.homepage.sections.find(section => section.type === 'artist')!.enabled = false
    expect(internalContentUrl(hiddenArtist, hiddenArtist.artist.id)).toBeNull()

    const hiddenSlide = structuredClone(snapshot)
    const slide = hiddenSlide.heroSlides[0]!
    slide.startsAt = '2028-01-01T00:00:00.000Z'
    expect(internalContentUrl(hiddenSlide, slide.id)).toBeNull()
  })
})
