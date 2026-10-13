import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { applyVideoDraft, createVideo, createVideoDraft, type VideoDraft } from '../../utils/video-workbench'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

const text = JSON.stringify(fixture)
/** 使用 canonical 分析结果建立可编辑副本。 */
function draft() { return createVideoDraft(analyzeSnapshotText(text).snapshot!) }

describe('影像应用事务', () => {
  it('深复制所有影像字段，仅替换 videos，保留其他内容', () => {
    const source = analyzeSnapshotText(text).snapshot!
    const value = createVideoDraft(source)
    value.videos[0]!.title['zh-CN'] = '新影像'
    value.videos[0]!.platformLinks[0]!.url = 'https://www.bilibili.com/video/BV123456'
    expect(source.videos).toEqual(fixture.videos)
    const result = applyVideoDraft(text, text, value)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text)
    expect(next).toEqual({ ...fixture, videos: value.videos })
  })
  it('新增记录生成唯一合法 ID，保留空必填内容待填写', () => {
    const first = createVideo()
    expect(first).toEqual({ id: expect.stringMatching(/^video_[a-z0-9_]+$/), title: { 'zh-CN': '' }, posterAssetId: '', durationSeconds: 1, featured: false, platformLinks: [{ provider: 'bilibili', url: '' }] })
    expect(createVideo().id).not.toBe(first.id)
    const value = draft()
    value.videos.push({ ...first, title: { 'zh-CN': '新增' }, posterAssetId: fixture.videos[0]!.posterAssetId, platformLinks: analyzeSnapshotText(text).snapshot!.videos[0]!.platformLinks })
    expect(applyVideoDraft(text, text, value).error).toBeNull()
  })
  it.each(['remove', 'rename'])('拒绝原记录 %s，失败保留原文', (action) => {
    const value = draft()
    if (action === 'remove') value.videos.shift()
    else value.videos[0]!.id = 'video_renamed'
    expect(applyVideoDraft(text, text, value)).toMatchObject({ text, error: 'immutable-video' })
  })
  it('重复 ID 与不合法新增 ID 交由完整诊断拒绝', () => {
    const value = draft()
    value.videos.push(structuredClone(value.videos[0]!))
    expect(applyVideoDraft(text, text, value)).toMatchObject({ text, error: 'invalid-draft' })
    value.videos.at(-1)!.id = 'bad id'
    expect(applyVideoDraft(text, text, value).issues.length).toBeGreaterThan(0)
  })
  it.each([
    ['posterAssetId', 'asset_preview_sample', '/videos/0/posterAssetId'],
    ['posterAssetId', 'asset_missing', '/videos/0/posterAssetId'],
    ['videoAssetId', 'asset_cover_01', '/videos/0/videoAssetId'],
    ['durationSeconds', 0, '/videos/0/durationSeconds'],
    ['durationSeconds', 1.5, '/videos/0/durationSeconds'],
    ['platformLinks', [], '/videos/0/platformLinks'],
    ['platformLinks', [{ provider: 'bilibili', url: 'javascript:alert(1)' }], '/videos/0/platformLinks/0/url'],
  ])('拒绝非法 %s，并返回准确路径', (key, invalid, path) => {
    const value = draft()
    Object.assign(value.videos[0]!, { [key]: invalid })
    const result = applyVideoDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path === path)).toBe(true)
    if (invalid === 'asset_missing') expect(result.issues).toContainEqual(expect.objectContaining({ path, code: 'missing-reference' }))
  })
  it('移除可选视频字段后仍可应用', () => {
    const value = draft()
    delete value.videos[0]!.videoAssetId
    expect(JSON.parse(applyVideoDraft(text, text, value).text).videos[0]).not.toHaveProperty('videoAssetId')
  })
  it('精确比较原文优先，当前 JSON 与 MIME 无效时不覆盖', () => {
    expect(applyVideoDraft(text + '\n', text, draft()).error).toBe('stale-snapshot')
    expect(applyVideoDraft('{', '{', draft())).toMatchObject({ text: '{', error: 'invalid-snapshot' })
    const invalid = structuredClone(fixture)
    invalid.assets[0]!.mimeType = 'video/mp4'
    const bad = JSON.stringify(invalid)
    expect(applyVideoDraft(bad, bad, draft()).error).toBe('invalid-snapshot')
  })
  it.each([null, {}, { videos: null }, { videos: [null] }, { videos: [1] }])('防御非法草稿形状 %j', (value) => {
    expect(applyVideoDraft(text, text, value as unknown as VideoDraft)).toMatchObject({ text, error: 'invalid-draft' })
  })
})
