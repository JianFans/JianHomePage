import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { applyMomentDraft, createMoment, createMomentDraft, type MomentDraft } from '../../utils/moment-workbench'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

const text = JSON.stringify(fixture)
/** 使用 canonical 快照建立独立草稿。 */
function draft() { return createMomentDraft(analyzeSnapshotText(text).snapshot!) }

describe('片段应用事务', () => {
  it('深复制所有字段且仅替换 moments', () => {
    const source = analyzeSnapshotText(text).snapshot!
    const value = createMomentDraft(source)
    value.moments[0]!.caption = { 'zh-CN': '修改片段' }
    expect(source.moments).toEqual(fixture.moments)
    const result = applyMomentDraft(text, text, value)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text)).toEqual({ ...fixture, moments: value.moments })
  })
  it('新增唯一 UUID ID、空素材且不编造选填字段', () => {
    const first = createMoment()
    expect(first).toEqual({ id: expect.stringMatching(/^moment_[a-z0-9_]+$/), assetId: '' })
    expect(createMoment().id).not.toBe(first.id)
    const value = draft()
    value.moments.push({ ...first, assetId: 'asset_cover_01' })
    expect(applyMomentDraft(text, text, value).error).toBeNull()
  })
  it.each(['remove', 'rename'])('拒绝已有 ID %s', (action) => {
    const value = draft()
    if (action === 'remove') value.moments.shift()
    else value.moments[0]!.id = 'moment_renamed'
    expect(applyMomentDraft(text, text, value)).toMatchObject({ text, error: 'immutable-moment' })
  })
  it('重复 ID 返回准确 canonical 诊断', () => {
    const value = draft()
    value.moments.push(structuredClone(value.moments[0]!))
    const result = applyMomentDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues).toContainEqual(expect.objectContaining({ path: `/moments/${value.moments.length - 1}/id`, code: 'duplicate-id' }))
  })
  it.each(['', 'asset_missing', 'asset_preview_sample'])('拒绝错误或空素材 %s 并保留原文', (assetId) => {
    const value = draft()
    value.moments[0]!.assetId = assetId
    const result = applyMomentDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path === '/moments/0/assetId')).toBe(true)
  })
  it('允许图片/GIF 素材与删除 caption、target、英文', () => {
    const source = structuredClone(fixture)
    source.assets.push({ ...source.assets[0]!, id: 'asset_gif_test', kind: 'gif', mimeType: 'image/gif', src: '/media/test.gif' })
    const baseline = JSON.stringify(source)
    const value = createMomentDraft(analyzeSnapshotText(baseline).snapshot!)
    for (const id of ['asset_cover_01', 'asset_gif_test']) {
      value.moments[0]!.assetId = id
      expect(applyMomentDraft(baseline, baseline, value).error).toBeNull()
    }
    value.moments[0]!.caption = { 'zh-CN': '中文', en: 'English' }
    delete value.moments[0]!.caption.en
    delete value.moments[0]!.caption
    delete value.moments[0]!.target
    const result = applyMomentDraft(baseline, baseline, value)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).moments[0]).toEqual({ id: value.moments[0]!.id, assetId: 'asset_gif_test' })
  })
  it.each(['http://example.com', 'javascript:alert(1)', ''])('拒绝非 HTTPS 外链 %s', (url) => {
    const value = draft()
    value.moments[0]!.target = { kind: 'external', link: { provider: 'website', url } }
    const result = applyMomentDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path.startsWith('/moments/0/target'))).toBe(true)
  })
  it.each(['missing_record', 'event_02'])('拒绝缺失或隐藏内部目标 %s', (contentId) => {
    const source = structuredClone(fixture)
    source.events[1]!.status = 'cancelled'
    const baseline = JSON.stringify(source)
    const value = createMomentDraft(analyzeSnapshotText(baseline).snapshot!)
    value.moments[0]!.target = { kind: 'internal', contentId }
    const result = applyMomentDraft(baseline, baseline, value)
    expect(result).toMatchObject({ text: baseline, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path.startsWith('/moments/0/target'))).toBe(true)
  })
  it('精确基线优先且非法原文返回诊断', () => {
    expect(applyMomentDraft(text + '\n', text, draft()).error).toBe('stale-snapshot')
    expect(applyMomentDraft('{', '{', draft())).toMatchObject({ text: '{', error: 'invalid-snapshot', issues: [{ path: '/', source: 'editor', code: 'invalid-json' }] })
    expect(applyMomentDraft('{}', '{}', draft()).error).toBe('invalid-snapshot')
  })
  it.each([null, {}, { moments: null }, { moments: [null] }, { moments: [1] }, { moments: [[]] }])('防御非法草稿 %j', (value) => {
    expect(applyMomentDraft(text, text, value as unknown as MomentDraft)).toMatchObject({ text, error: 'invalid-draft' })
  })
})
