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
    videoSection.itemIds = ['video_missing', 'video_03', 'video_01']
    videoSection.limit = 1

    const visible = resolveHomepageSections(changed, 'zh-CN', now)
    const videos = visible.find(section => section.type === 'video')

    expect(videos?.items.map(video => video.id)).toEqual(['video_03'])
  })

  it('现场只保留未来且状态为 scheduled 的条目', () => {
    const changed = structuredClone(snapshot)
    changed.events[0]!.dateTime = '2026-08-28T23:59:59Z'
    changed.events[1]!.status = 'sold-out'

    const visible = resolveHomepageSections(changed, 'en', now)

    expect(visible.some(section => section.type === 'event')).toBe(false)
  })
})
