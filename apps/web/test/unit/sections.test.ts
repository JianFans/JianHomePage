import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json'
import { resolveHomepageSections } from '../../utils/sections'

const snapshot = fixture as unknown as YujianContentSnapshot
const now = new Date('2026-08-29T00:00:00Z')

describe('首页板块解析', () => {
  it('保持后台顺序并返回所有有内容的启用板块', () => {
    const visible = resolveHomepageSections(snapshot, 'zh-CN', now)

    expect(visible.map(section => section.type)).toEqual([
      'hero',
      'music',
      'video',
      'event',
      'moment',
      'artist',
    ])
  })

  it('隐藏未启用和没有可显示内容的板块', () => {
    const changed = structuredClone(snapshot)
    const videoSection = changed.homepage.sections.find(section => section.type === 'video')!
    videoSection.enabled = false
    changed.events = []

    const visible = resolveHomepageSections(changed, 'zh-CN', now)

    expect(visible.some(section => section.type === 'video')).toBe(false)
    expect(visible.some(section => section.type === 'event')).toBe(false)
  })

  it('跳过缺失内容 ID 并遵守板块数量上限', () => {
    const changed = structuredClone(snapshot)
    const videoSection = changed.homepage.sections.find(section => section.type === 'video')!
    videoSection.itemIds = ['video_b2c95663-9e0f-4947-8325-365131897183', 'video_8ba6c435-f5b3-4603-a43e-f326ba0a12a5', 'video_b634e94d-b468-4e4b-b784-22eda2c49314']
    videoSection.limit = 1

    const visible = resolveHomepageSections(changed, 'zh-CN', now)
    const videos = visible.find(section => section.type === 'video')

    expect(videos?.items.map(video => video.id)).toEqual(['video_8ba6c435-f5b3-4603-a43e-f326ba0a12a5'])
  })

  it('现场只保留未来且状态为 scheduled 的条目', () => {
    const changed = structuredClone(snapshot)
    changed.events[0]!.dateTime = '2026-08-28T23:59:59Z'
    changed.events[1]!.status = 'sold-out'

    const visible = resolveHomepageSections(changed, 'en', now)

    expect(visible.some(section => section.type === 'event')).toBe(false)
  })

  it('Hero 只保留在展示时间窗内的条目', () => {
    const changed = structuredClone(snapshot)
    changed.heroSlides[0]!.startsAt = now.toISOString()
    changed.heroSlides[1]!.startsAt = '2026-08-29T00:00:01Z'
    changed.heroSlides[2]!.endsAt = now.toISOString()

    const visible = resolveHomepageSections(changed, 'zh-CN', now)
    const hero = visible.find(section => section.type === 'hero')

    expect(hero?.items.map(item => item.id)).toEqual(['hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb'])
  })

  it('Hero 条目全部不在展示时间窗时隐藏板块', () => {
    const changed = structuredClone(snapshot)
    changed.heroSlides.forEach((slide) => {
      slide.startsAt = '2026-08-29T00:00:01Z'
    })

    const visible = resolveHomepageSections(changed, 'zh-CN', now)

    expect(visible.some(section => section.type === 'hero')).toBe(false)
  })

  it('默认使用快照生成时间保持静态渲染与 hydration 一致', () => {
    const changed = structuredClone(snapshot)
    changed.generatedAt = '1999-12-31T00:00:00Z'
    changed.events[0]!.dateTime = '2000-01-01T00:00:00Z'
    changed.events = [changed.events[0]!]

    const visible = resolveHomepageSections(changed, 'zh-CN')

    expect(visible.find(section => section.type === 'event')?.items).toHaveLength(1)
  })
})
