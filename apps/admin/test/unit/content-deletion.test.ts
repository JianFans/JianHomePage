import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { analyzeContentDeletion, applyContentDeletion } from '../../utils/content-deletion'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

const original = JSON.stringify(fixture)
const releaseId = fixture.releases[1]!.id
const trackId = fixture.tracks[1]!.id

describe('引用安全删除事务', () => {
  it('作品连同所属曲目删除，显式列出并清理全部重复编排，保留素材和其他内容', () => {
    const source = structuredClone(fixture)
    source.homepage.sections[1]!.itemIds.push(releaseId, releaseId)
    const text = JSON.stringify(source)
    const plan = analyzeContentDeletion(text, 'release', releaseId)
    expect(plan.error).toBeNull()
    expect(plan.removed.map(item => item.path)).toEqual(['/releases/1', '/tracks/1'])
    expect(plan.references).toEqual(expect.arrayContaining([
      { path: '/homepage/sections/1/itemIds/1', contentId: releaseId, action: 'arrangement' },
      { path: '/homepage/sections/1/itemIds/5', contentId: releaseId, action: 'arrangement' },
      { path: '/homepage/sections/1/itemIds/6', contentId: releaseId, action: 'arrangement' },
      { path: '/releases/1/trackIds/0', contentId: trackId, action: 'cascade' },
      { path: '/tracks/1/releaseId', contentId: releaseId, action: 'cascade' },
    ]))
    const result = applyContentDeletion(text, plan)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text)
    expect(next.releases).toEqual(source.releases.filter(item => item.id !== releaseId))
    expect(next.tracks).toEqual(source.tracks.filter(item => item.id !== trackId))
    expect(next.homepage.sections.map((section: { itemIds: string[] }) => section.itemIds)).toEqual(
      source.homepage.sections.map(section => section.itemIds.filter(id => id !== releaseId)),
    )
    expect(source.homepage.sections[1]!.itemIds).toContain(releaseId)
    expect(next.homepage.sections[1].itemIds).toContain(source.releases[0]!.id)
    expect(next.assets).toEqual(source.assets)
    expect(next.videos).toEqual(source.videos)
    expect(analyzeSnapshotText(result.text).snapshot).not.toBeNull()
    expect(source.releases).toHaveLength(5)
  })

  it('列出宣传位关联和所有内部跳转路径并阻止删除，不清空字段', () => {
    const source = structuredClone(fixture)
    const id = source.releases[0]!.id
    Object.assign(source.moments[0]!, { target: { kind: 'internal', contentId: id } })
    const text = JSON.stringify(source)
    const plan = analyzeContentDeletion(text, 'release', id)
    expect(plan.error).toBe('blocked-reference')
    expect(plan.references.filter(item => item.action === 'blocking').map(item => item.path)).toEqual([
      '/heroSlides/2/releaseId', '/heroSlides/2/target/contentId', '/moments/0/target/contentId',
    ])
    expect(applyContentDeletion(text, plan)).toMatchObject({ text, error: 'blocked-reference' })
  })

  it('单曲删除移除所属作品的全部重复引用，最后一首不同曲目不可删除', () => {
    expect(analyzeContentDeletion(original, 'track', trackId).error).toBe('last-track')
    const source = structuredClone(fixture)
    const extra = { ...source.tracks[1]!, id: 'track_79df0a45-4eb4-4fe7-bb44-a484d40f941a' }
    source.tracks.push(extra)
    source.releases[1]!.trackIds.push(trackId, extra.id)
    const text = JSON.stringify(source)
    const plan = analyzeContentDeletion(text, 'track', trackId)
    expect(plan.error).toBeNull()
    expect(plan.references.filter(item => item.action === 'ownership').map(item => item.path)).toEqual([
      '/releases/1/trackIds/0', '/releases/1/trackIds/1',
    ])
    const result = applyContentDeletion(text, plan)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).releases[1].trackIds).toEqual([extra.id])
    source.releases[1]!.trackIds = [trackId, trackId]
    expect(analyzeContentDeletion(JSON.stringify(source), 'track', trackId).error).toBe('last-track')
  })

  it.each([
    ['video', 'videos', 2], ['event', 'events', 3], ['moment', 'moments', 4], ['hero', 'heroSlides', 0],
  ] as const)('删除 %s 及编排引用，保留其他集合', (kind, key, section) => {
    const id = fixture[key][0]!.id
    const plan = analyzeContentDeletion(original, kind, id)
    const result = applyContentDeletion(original, plan)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text)
    expect(next[key]).toEqual(fixture[key].slice(1))
    expect(next.homepage.sections.map((item: { itemIds: string[] }) => item.itemIds)).toEqual(
      fixture.homepage.sections.map(item => item.itemIds.filter(itemId => itemId !== id)),
    )
    expect(fixture.homepage.sections[section]!.itemIds).toContain(id)
    expect(next.homepage.sections[section].itemIds).toContain(fixture[key][1]!.id)
    expect(next.assets).toEqual(fixture.assets)
  })

  it('最新文本与基线不一致、无效文本和不存在目标均保留原文', () => {
    const plan = analyzeContentDeletion(original, 'video', fixture.videos[0]!.id)
    expect(applyContentDeletion(original + '\n', plan)).toMatchObject({ text: original + '\n', error: 'stale-snapshot' })
    expect(analyzeContentDeletion('{', 'video', fixture.videos[0]!.id)).toMatchObject({ error: 'invalid-snapshot', issues: [expect.objectContaining({ code: 'invalid-json' })] })
    expect(applyContentDeletion('{', { ...plan, baseline: '{' })).toMatchObject({ text: '{', error: 'invalid-snapshot' })
    expect(analyzeContentDeletion(original, 'video', 'missing').error).toBe('missing-target')
  })

  it('执行时重新分析，不信任调用者篡改的引用或预检查结果', () => {
    const plan = analyzeContentDeletion(original, 'release', fixture.releases[0]!.id)
    expect(applyContentDeletion(original, { ...plan, error: null, references: [] })).toMatchObject({ text: original, error: 'blocked-reference' })
  })

  it('删除未被引用的宣传位仍须检查剩余内部跳转能否生成锚点', () => {
    const source = structuredClone(fixture)
    // A moment points to the selected hero; this must not be silently cleared.
    Object.assign(source.moments[0]!, { target: { kind: 'internal', contentId: source.heroSlides[0]!.id } })
    const text = JSON.stringify(source)
    expect(analyzeSnapshotText(text).snapshot).not.toBeNull()
    const plan = analyzeContentDeletion(text, 'hero', source.heroSlides[0]!.id)
    expect(plan.error).toBe('blocked-reference')
    expect(applyContentDeletion(text, plan).text).toBe(text)
  })
})
