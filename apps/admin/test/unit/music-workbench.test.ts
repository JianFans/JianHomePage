import { describe, expect, it } from 'vitest'
import { diagnoseContentSnapshot, type YujianContentSnapshot } from '@yujian/schema'
import fixtureData from '../../../../content/fixtures/homepage.json'
import { applyMusicDraft, createMusicDraft, createMusicTrack, musicAssets } from '../../utils/music-workbench'

/** 每个场景使用独立快照，避免引用修改污染其他测试。 */
function snapshot(): YujianContentSnapshot {
  return structuredClone(fixtureData) as unknown as YujianContentSnapshot
}

/** 构造包含最少必填字段的新作品表单。 */
function newDraft(value = snapshot()) {
  const draft = createMusicDraft(value, null)!
  draft.release.title = { 'zh-CN': '新作品', en: 'New release' }
  draft.release.releaseDate = '2026-10-10'
  draft.release.coverAssetId = value.releases[0]!.coverAssetId
  draft.tracks[0]!.title = { 'zh-CN': '新曲目' }
  draft.tracks[0]!.durationSeconds = 180
  return draft
}

describe('音乐快照事务', () => {
  it('一次性加入作品与曲目并保留无关内容', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = newDraft(value)
    draft.sections[0]!.itemIds.unshift(draft.release.id)
    const result = applyMusicDraft(text, text, draft)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text) as YujianContentSnapshot
    expect(diagnoseContentSnapshot(next)).toEqual([])
    expect(next.releases.at(-1)).toMatchObject({ ...draft.release, trackIds: [draft.tracks[0]!.id] })
    expect(next.tracks.at(-1)?.releaseId).toBe(draft.release.id)
    expect(next.heroSlides).toEqual(value.heroSlides)
    expect(next.assets).toEqual(value.assets)
    expect(value.releases).toHaveLength(fixtureData.releases.length)
  })

  it('按作品曲目顺序打开表单且不共享引用', () => {
    const value = snapshot()
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.release.title['zh-CN'] = '改名'
    draft.tracks[0]!.credits[0]!.name = '新署名'
    expect(value.releases[0]!.title['zh-CN']).not.toBe('改名')
    expect(value.tracks[0]!.credits[0]!.name).not.toBe('新署名')
    expect(createMusicDraft(value, 'release_missing')).toBeNull()
  })

  it('生成符合前缀的新 ID 且新曲目归属当前作品', () => {
    const value = snapshot()
    const draft = createMusicDraft(value, null)!
    const second = createMusicTrack(draft.release.id)
    expect(draft.release.id).toMatch(/^release_/)
    expect(second.id).toMatch(/^track_/)
    expect(second.id).not.toBe(draft.tracks[0]!.id)
    expect(second.releaseId).toBe(draft.release.id)
  })

  it('只提供适合封面和试听的素材', () => {
    const value = snapshot()
    expect(musicAssets(value, 'cover').every(asset => ['image', 'gif'].includes(asset.kind))).toBe(true)
    expect(musicAssets(value, 'preview').map(asset => asset.id)).toContain('asset_preview_sample')
    expect(musicAssets(value, 'preview').every(asset => asset.kind === 'audio')).toBe(true)
  })

  it('更新现有作品、重排曲目并保留孤立但合法的曲目记录', () => {
    const value = snapshot()
    const orphan = { ...structuredClone(value.tracks[0]!), id: 'track_orphan' }
    value.tracks.push(orphan)
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    const second = { ...structuredClone(draft.tracks[0]!), id: 'track_added' }
    draft.tracks.unshift(second)
    draft.release.title['zh-CN'] = '新标题'
    const result = applyMusicDraft(text, text, draft)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text) as YujianContentSnapshot
    expect(next.releases[0]!.trackIds).toEqual(['track_added', 'track_01'])
    expect(next.tracks.find(track => track.id === orphan.id)).toEqual(orphan)
    expect(next.releases[1]).toEqual(value.releases[1])
  })

  it('移除无外部引用的曲目并保留素材', () => {
    const value = snapshot()
    const extra = { ...structuredClone(value.tracks[0]!), id: 'track_extra' }
    value.tracks.push(extra)
    value.releases[0]!.trackIds.push(extra.id)
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.tracks.pop()
    const result = applyMusicDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).tracks.some((track: { id: string }) => track.id === extra.id)).toBe(false)
    expect(JSON.parse(result.text).assets).toEqual(value.assets)
  })

  it('曲目仍被内部目标引用时阻止移除', () => {
    const value = snapshot()
    value.heroSlides[0]!.target = { kind: 'internal', contentId: value.tracks[0]!.id }
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.tracks = [{ ...draft.tracks[0]!, id: 'track_replacement' }]
    const result = applyMusicDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.code === 'missing-reference')).toBe(true)
  })

  it('音乐条目移出展示范围导致内部目标隐藏时阻止应用', () => {
    const value = snapshot()
    value.heroSlides[0]!.target = { kind: 'internal', contentId: value.releases[0]!.id }
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.sections[0]!.itemIds.shift()
    const result = applyMusicDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.code === 'hidden-target')).toBe(true)
  })

  it('阻止过期表单覆盖当前文本', () => {
    const text = JSON.stringify(snapshot())
    const result = applyMusicDraft(`${text}\n`, text, newDraft())
    expect(result).toMatchObject({ text: `${text}\n`, error: 'stale-snapshot' })
  })

  it.each(['{', '{}', '[]'])('无效基线 %s 保持原文', (text) => {
    expect(applyMusicDraft(text, text, newDraft())).toMatchObject({ text, error: 'invalid-snapshot' })
  })

  it('拒绝重复作品 ID、重复曲目和借用其他作品的曲目 ID', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const duplicate = newDraft(value)
    duplicate.release.id = value.releases[0]!.id
    expect(applyMusicDraft(text, text, duplicate).error).toBe('duplicate-id')
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.tracks.push(structuredClone(draft.tracks[0]!))
    expect(applyMusicDraft(text, text, draft).error).toBe('duplicate-id')
    draft.tracks = [structuredClone(value.tracks[1]!)]
    expect(applyMusicDraft(text, text, draft).error).toBe('track-ownership')
  })

  it('拒绝作品改名 ID 和不存在的原作品', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.release.id = 'release_renamed'
    expect(applyMusicDraft(text, text, draft).error).toBe('immutable-id')
    draft.originalId = 'release_missing'
    expect(applyMusicDraft(text, text, draft).error).toBe('missing-release')
  })

  it.each(['cover', 'date', 'url', 'duration', 'empty-tracks'])('无效字段 %s 不改写文本', (field) => {
    const text = JSON.stringify(snapshot())
    const draft = newDraft()
    if (field === 'cover') draft.release.coverAssetId = 'asset_preview_sample'
    if (field === 'date') draft.release.releaseDate = '2026-02-30'
    if (field === 'url') draft.release.platformLinks = [{ provider: 'website', url: 'javascript:alert(1)' }]
    if (field === 'duration') draft.tracks[0]!.durationSeconds = 0
    if (field === 'empty-tracks') draft.tracks = []
    expect(applyMusicDraft(text, text, draft)).toMatchObject({ text, error: 'invalid-draft' })
  })

  it('保留音乐板块启用状态、限制和更多链接', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createMusicDraft(value, value.releases[0]!.id)!
    draft.sections[0]!.itemIds.reverse()
    const result = applyMusicDraft(text, text, draft)
    const next = JSON.parse(result.text) as YujianContentSnapshot
    expect(next.homepage.sections[1]).toEqual({ ...value.homepage.sections[1], itemIds: draft.sections[0]!.itemIds })
    draft.sections[0]!.id = 'section_unknown'
    expect(applyMusicDraft(text, text, draft)).toMatchObject({ text, error: 'invalid-draft' })
  })
})
