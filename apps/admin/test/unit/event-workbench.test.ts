import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { applyEventDraft, createEvent, createEventDraft, type EventDraft } from '../../utils/event-workbench'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

const text = JSON.stringify(fixture)
/** 以 canonical 结果创建独立草稿。 */
function draft() { return createEventDraft(analyzeSnapshotText(text).snapshot!) }

describe('现场应用事务', () => {
  it('深复制全部字段，只替换 events 并保留合法时区原文', () => {
    const source = analyzeSnapshotText(text).snapshot!
    const value = createEventDraft(source)
    value.events[0]!.title['zh-CN'] = '新现场'
    value.events[0]!.city['zh-CN'] = '新城市'
    value.events[0]!.venue['zh-CN'] = '新场馆'
    value.events[0]!.dateTime = '2030-01-02T20:00:00+08:00'
    expect(source.events).toEqual(fixture.events)
    const result = applyEventDraft(text, text, value)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text)).toEqual({ ...fixture, events: value.events })
  })
  it('创建唯一合法 ID 与空必填值，不设置选填海报', () => {
    const first = createEvent()
    expect(first).toEqual({ id: expect.stringMatching(/^event_[a-z0-9_]+$/), title: { 'zh-CN': '' }, city: { 'zh-CN': '' }, venue: { 'zh-CN': '' }, dateTime: '', status: 'scheduled', detailUrl: '' })
    expect(createEvent().id).not.toBe(first.id)
    const value = draft()
    value.events.push({ ...value.events[0]!, id: first.id })
    expect(applyEventDraft(text, text, value).error).toBeNull()
  })
  it.each(['remove', 'rename'])('拒绝原记录 %s 并保留原文', (action) => {
    const value = draft()
    if (action === 'remove') value.events.shift()
    else value.events[0]!.id = 'event_renamed'
    expect(applyEventDraft(text, text, value)).toMatchObject({ text, error: 'immutable-event' })
  })
  it('重复 ID 交由完整诊断拒绝', () => {
    const value = draft()
    value.events.push(structuredClone(value.events[0]!))
    const result = applyEventDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues).toContainEqual(expect.objectContaining({ path: '/events/2/id', code: 'duplicate-id' }))
  })
  it.each([
    ['dateTime', '2030-01-02T20:00:00', '/events/0/dateTime'],
    ['dateTime', '2030-13-02T20:00:00Z', '/events/0/dateTime'],
    ['dateTime', '2030-02-30T20:00:00Z', '/events/0/dateTime'],
    ['dateTime', '2030-01-02T24:00:00Z', '/events/0/dateTime'],
    ['status', 'unknown', '/events/0/status'],
    ['detailUrl', '', '/events/0/detailUrl'],
    ['detailUrl', 'javascript:alert(1)', '/events/0/detailUrl'],
    ['detailUrl', 'http://example.com/event', '/events/0/detailUrl'],
    ['posterAssetId', 'asset_preview_sample', '/events/0/posterAssetId'],
    ['posterAssetId', 'asset_missing', '/events/0/posterAssetId'],
  ])('拒绝非法 %s，保留输入并返回准确路径', (key, invalid, path) => {
    const value = draft()
    Object.assign(value.events[0]!, { [key]: invalid })
    const result = applyEventDraft(text, text, value)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path === path)).toBe(true)
  })
  it('支持图片、GIF 海报与选填字段删除', () => {
    const source = structuredClone(fixture)
    source.assets.push({ ...source.assets[0]!, id: 'asset_gif_test', kind: 'gif', mimeType: 'image/gif', src: '/media/test.gif' })
    const baseline = JSON.stringify(source)
    const value = createEventDraft(analyzeSnapshotText(baseline).snapshot!)
    for (const id of ['asset_cover_01', 'asset_gif_test']) {
      value.events[0]!.posterAssetId = id
      expect(applyEventDraft(baseline, baseline, value).error).toBeNull()
    }
    delete value.events[0]!.posterAssetId
    delete value.events[0]!.title.en
    delete value.events[0]!.city.en
    delete value.events[0]!.venue.en
    const result = applyEventDraft(baseline, baseline, value)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).events[0]).not.toHaveProperty('posterAssetId')
  })
  it.each(['cancelled', 'past'])('拒绝 %s 导致已有内部跳转目标隐藏，不删引用', (change) => {
    const source = structuredClone(fixture)
    Object.assign(source.heroSlides[0]!, { target: { kind: 'internal', contentId: 'event_01' } })
    const baseline = JSON.stringify(source)
    expect(analyzeSnapshotText(baseline).snapshot).not.toBeNull()
    const value = createEventDraft(analyzeSnapshotText(baseline).snapshot!)
    if (change === 'past') value.events[0]!.dateTime = '2000-01-01T00:00:00Z'
    else value.events[0]!.status = 'cancelled'
    const result = applyEventDraft(baseline, baseline, value)
    expect(result).toMatchObject({ text: baseline, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path.startsWith('/heroSlides/0/'))).toBe(true)
  })
  it('精确基线优先，当前非法快照返回原文与诊断', () => {
    expect(applyEventDraft(text + '\n', text, draft()).error).toBe('stale-snapshot')
    expect(applyEventDraft('{', '{', draft())).toMatchObject({ text: '{', error: 'invalid-snapshot', issues: [{ path: '/', source: 'editor', code: 'invalid-json' }] })
    expect(applyEventDraft('{}', '{}', draft()).error).toBe('invalid-snapshot')
  })
  it.each([null, {}, { events: null }, { events: [null] }, { events: [1] }, { events: [[]] }])('防御非法草稿形状 %j', (value) => {
    expect(applyEventDraft(text, text, value as unknown as EventDraft)).toMatchObject({ text, error: 'invalid-draft' })
  })
})
