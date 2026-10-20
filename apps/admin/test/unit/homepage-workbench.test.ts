import { describe, expect, it } from 'vitest'
import { diagnoseContentSnapshot, type YujianContentSnapshot } from '@yujian/schema'
import fixtureData from '../../../../content/fixtures/homepage.json'
import { resolveHomepageSections } from '../../../web/utils/sections'
import {
  applyHomepageDraft, createHeroSlide, createHomepageDraft, homepageItemOptions,
  homepageItemVisibility, removeHomepageHero, type HomepageDraft,
} from '../../utils/homepage-workbench'

/** 每个测试使用独立的合法快照。 */
function snapshot(): YujianContentSnapshot {
  return structuredClone(fixtureData) as unknown as YujianContentSnapshot
}

/** 从当前草稿取得带判别类型的板块。 */
function section<T extends HomepageDraft['sections'][number]['type']>(draft: HomepageDraft, type: T) {
  return draft.sections.find(item => item.type === type)! as Extract<HomepageDraft['sections'][number], { type: T }>
}

describe('首页草稿事务', () => {
  it('深复制宣传位、嵌套字段与板块', () => {
    const value = snapshot()
    const draft = createHomepageDraft(value)
    draft.heroSlides[0]!.headline['zh-CN'] = '新标题'
    draft.heroSlides[0]!.focalPoint.x = 20
    section(draft, 'music').moreLink!.url = 'https://example.com'
    section(draft, 'hero').itemIds.reverse()
    expect(value).toEqual(fixtureData)
  })

  it('生成随机前缀 ID 与空必填字段', () => {
    const slide = createHeroSlide()
    expect(slide).toEqual({
      id: expect.stringMatching(/^hero_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/), mediaKind: 'image', assetId: '',
      headline: { 'zh-CN': '' }, focalPoint: { x: 50, y: 50 }, autoplay: false,
    })
    expect(createHeroSlide().id).not.toBe(slide.id)
  })

  it('新增宣传位并加入已有板块，只替换首页字段且不修改输入', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    const slide = createHeroSlide()
    slide.headline = { 'zh-CN': '新宣传位', en: 'New hero' }
    slide.assetId = value.heroSlides[0]!.assetId
    draft.heroSlides.push(slide)
    ;(section(draft, 'hero').itemIds as string[]).push(slide.id)
    const before = structuredClone(draft)
    const result = applyHomepageDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(result.issues).toEqual([])
    const next = JSON.parse(result.text) as YujianContentSnapshot
    expect(diagnoseContentSnapshot(next)).toEqual([])
    expect(next.heroSlides).toEqual(draft.heroSlides)
    expect(next.homepage.sections).toEqual(draft.sections)
    expect({ ...next, heroSlides: value.heroSlides, homepage: value.homepage }).toEqual(value)
    expect(draft).toEqual(before)
    expect(value).toEqual(fixtureData)
  })

  it('移除宣传位及所有 hero 板块的重复引用，保留媒体与其他目标', () => {
    const value = snapshot()
    const id = value.heroSlides[0]!.id
    value.moments[0]!.target = { kind: 'internal', contentId: id }
    const draft = createHomepageDraft(value)
    ;(section(draft, 'hero').itemIds as string[]).push(id)
    draft.sections.push({ ...structuredClone(section(draft, 'hero')), id: 'section_9eda8021-b0a8-46c6-8901-ce6e90e22203' })
    removeHomepageHero(draft, id)
    expect(draft.heroSlides.some(slide => slide.id === id)).toBe(false)
    expect(draft.sections.filter(item => item.type === 'hero').every(item => !(item.itemIds as string[]).includes(id))).toBe(true)
    expect(value.assets).toEqual(fixtureData.assets)
    expect(value.moments[0]!.target).toEqual({ kind: 'internal', contentId: id })
    expect(draft.sections.filter(item => item.type !== 'hero')).toEqual(value.homepage.sections.filter(item => item.type !== 'hero'))
  })

  it('删除无外部目标的宣传位可应用且保留素材', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    removeHomepageHero(draft, value.heroSlides[0]!.id)
    const result = applyHomepageDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).assets).toEqual(value.assets)
  })

  it('允许排序、启用、条目、上限和音乐更多链接编辑并保留重复引用', () => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    draft.sections.reverse()
    section(draft, 'video').enabled = false
    section(draft, 'music').itemIds = ['release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff', 'release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff', 'release_f9462a63-70d9-40b3-a0cc-a792f1b5499e']
    section(draft, 'music').limit = 2
    section(draft, 'music').moreLink = { provider: 'website', url: 'https://example.com/music' }
    const result = applyHomepageDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).homepage.sections).toEqual(draft.sections)
  })

  it.each(['id', 'type', 'layoutVariant', 'remove', 'add', 'duplicate'] as const)('拒绝改变板块身份或布局：%s', (change) => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    if (change === 'id') draft.sections[0]!.id = 'section_2eead546-1cb4-41e5-a5ca-9e39ea66b1de'
    if (change === 'type') Object.assign(draft.sections[0]!, { type: 'artist', layoutVariant: 'split', itemIds: [] })
    if (change === 'layoutVariant') Object.assign(draft.sections[0]!, { layoutVariant: 'split' })
    if (change === 'remove') draft.sections.pop()
    if (change === 'add') draft.sections.push({ ...structuredClone(draft.sections[0]!), id: 'section_065874b5-3b0e-4122-83d5-f30d82e60744' })
    if (change === 'duplicate') draft.sections[1] = structuredClone(draft.sections[0]!)
    const before = structuredClone(draft)
    expect(applyHomepageDraft(text, text, draft)).toMatchObject({ text, error: 'immutable-section' })
    expect(draft).toEqual(before)
  })

  it.each(['hero', 'artist'] as const)('保留 canonical 允许的同 ID 板块并允许重排：%s', (type) => {
    const value = snapshot()
    const copy = structuredClone(value.homepage.sections.find(item => item.type === type)!)
    copy.id = value.homepage.sections[0].id
    value.homepage.sections.push(copy)
    expect(diagnoseContentSnapshot(value)).toEqual([])
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    expect(applyHomepageDraft(text, text, draft).error).toBeNull()
    draft.sections.reverse()
    const result = applyHomepageDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).homepage.sections).toEqual(draft.sections)
  })

  it.each(['remove', 'add', 'replace'] as const)('按身份三元组计数，拒绝改变已有重复板块：%s', (change) => {
    const value = snapshot()
    value.homepage.sections.push(structuredClone(value.homepage.sections[0]))
    expect(diagnoseContentSnapshot(value)).toEqual([])
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    if (change === 'remove') draft.sections.pop()
    if (change === 'add') draft.sections.push(structuredClone(draft.sections[0]))
    if (change === 'replace') {
      draft.sections[draft.sections.length - 1] = { ...structuredClone(section(draft, 'artist')), id: draft.sections[0].id }
    }
    const before = structuredClone(draft)
    expect(applyHomepageDraft(text, text, draft)).toMatchObject({ text, error: 'immutable-section' })
    expect(draft).toEqual(before)
  })

  it.each([6])('hero 引用数量 %s 失败且保留原文', (count) => {
    const value = snapshot()
    const text = JSON.stringify(value, null, 4)
    const draft = createHomepageDraft(value)
    Object.assign(section(draft, 'hero'), { itemIds: Array.from({ length: count }, () => 'hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb') })
    const before = structuredClone(draft)
    const result = applyHomepageDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path.startsWith('/homepage/sections/0/itemIds'))).toBe(true)
    expect(draft).toEqual(before)
  })

  it.each([0, 1, 5])('允许 hero 引用边界 %s，重复引用不去重', (count) => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    Object.assign(section(draft, 'hero'), { itemIds: Array.from({ length: count }, () => 'hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb') })
    const result = applyHomepageDraft(text, text, draft)
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).homepage.sections[0].itemIds).toHaveLength(count)
  })

  it.each(['assetId', 'mobileAssetId', 'posterAssetId'] as const)('拒绝错误媒体用途：%s', (field) => {
    const value = snapshot()
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    draft.heroSlides[0]![field] = 'asset_71734f61-5250-4825-a8b3-0192448be437'
    const result = applyHomepageDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.path === `/heroSlides/0/${field}` && issue.source === 'semantic')).toBe(true)
  })

  it.each(['disabled', 'limit', 'removed', 'time-window'] as const)('完整校验拒绝隐藏内部目标：%s', (change) => {
    const value = snapshot()
    if (change === 'time-window') value.moments[0]!.target = { kind: 'internal', contentId: 'hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb' }
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    if (change === 'disabled') section(draft, 'music').enabled = false
    if (change === 'limit') { section(draft, 'music').itemIds.reverse(); section(draft, 'music').limit = 1 }
    if (change === 'removed') section(draft, 'music').itemIds.shift()
    if (change === 'time-window') draft.heroSlides[0]!.endsAt = value.generatedAt
    const result = applyHomepageDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues.some(issue => issue.code === 'hidden-target')).toBe(true)
  })

  it('删除仍被片段目标引用的宣传位时不清理目标而拒绝应用', () => {
    const value = snapshot()
    value.moments[0]!.target = { kind: 'internal', contentId: 'hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb' }
    const text = JSON.stringify(value)
    const draft = createHomepageDraft(value)
    removeHomepageHero(draft, 'hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb')
    const result = applyHomepageDraft(text, text, draft)
    expect(result).toMatchObject({ text, error: 'invalid-draft' })
    expect(result.issues).toContainEqual({ path: '/moments/0/target/contentId', source: 'semantic', code: 'missing-reference' })
  })

  it('先检查基线，禁止旧草稿覆盖最新文本', () => {
    expect(applyHomepageDraft('{', 'old', createHomepageDraft(snapshot()))).toEqual({ text: '{', error: 'stale-snapshot', issues: [] })
  })

  it.each(['{', '{}', '[]'])('完整校验原始文本 %s 并保留失败原文', (text) => {
    const result = applyHomepageDraft(text, text, createHomepageDraft(snapshot()))
    expect(result).toMatchObject({ text, error: 'invalid-snapshot' })
    expect(result.issues.length).toBeGreaterThan(0)
  })

  it('不允许用有效首页草稿覆盖其他模块已经无效的快照', () => {
    const value = snapshot()
    const draft = createHomepageDraft(value)
    value.tracks[0]!.previewAssetId = 'asset_773e3dac-5b2c-40b5-83d6-1eaa5f4def9c'
    const text = JSON.stringify(value)
    expect(applyHomepageDraft(text, text, draft)).toMatchObject({ text, error: 'invalid-snapshot' })
  })

  it.each([null, {}, { heroSlides: null, sections: null }, { heroSlides: [null], sections: [null] }])('异常草稿不崩溃并保留原文', (draft) => {
    const text = JSON.stringify(snapshot())
    expect(applyHomepageDraft(text, text, draft as unknown as HomepageDraft)).toMatchObject({ text, error: 'invalid-draft' })
  })
})

describe('首页条目选项与逐位置可见性', () => {
  it('从正确集合取双语标题，hero 使用草稿，片段无标题时回退 ID', () => {
    const value = snapshot()
    delete value.moments[0]!.caption
    const draft = createHomepageDraft(value)
    draft.heroSlides[0]!.headline = { 'zh-CN': '草稿标题', en: 'Draft' }
    const sources = {
      hero: draft.heroSlides.map(item => ({ id: item.id, title: item.headline })),
      music: value.releases.map(item => ({ id: item.id, title: item.title })),
      video: value.videos.map(item => ({ id: item.id, title: item.title })),
      event: value.events.map(item => ({ id: item.id, title: item.title })),
      moment: value.moments.map(item => ({ id: item.id, title: item.caption ?? { 'zh-CN': item.id } })),
      artist: [{ id: value.artist.id, title: value.artist.name }],
    }
    for (const item of draft.sections) expect(homepageItemOptions(value, draft, item)).toEqual(sources[item.type])
  })

  it('与公开站解析逐板块一致，重复引用独立占用上限', () => {
    const value = snapshot()
    const draft = createHomepageDraft(value)
    for (const item of draft.sections) {
      if ('limit' in item) {
        item.itemIds = [item.itemIds[0]!, item.itemIds[0]!, ...item.itemIds.slice(1)]
        item.limit = 2
      }
    }
    const next = { ...value, heroSlides: draft.heroSlides, homepage: { sections: draft.sections } }
    const resolved = resolveHomepageSections(next, 'zh-CN')
    for (const item of draft.sections) {
      const states = homepageItemVisibility(value, draft, item)
      expect(item.itemIds.filter((_, index) => states[index] === 'visible')).toEqual(resolved.find(result => result.id === item.id)?.items.map(content => content.id) ?? [])
      if ('limit' in item) expect(states).toEqual(item.itemIds.map((_, index) => index < 2 ? 'visible' : 'limit'))
    }
  })

  it('hero 起点包含、终点排除，使用 generatedAt 和草稿而非系统时间', () => {
    const value = snapshot()
    const draft = createHomepageDraft(value)
    draft.heroSlides[0]!.startsAt = value.generatedAt
    draft.heroSlides[1]!.endsAt = value.generatedAt
    draft.heroSlides[2]!.startsAt = '2026-08-30T00:00:00Z'
    const hero = section(draft, 'hero')
    expect(homepageItemVisibility(value, draft, hero)).toEqual(['visible', 'time-window', 'time-window'])
    const resolved = resolveHomepageSections({ ...value, heroSlides: draft.heroSlides }, 'en')
    expect(resolved.find(item => item.type === 'hero')?.items.map(item => item.id)).toEqual(['hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb'])
    hero.enabled = false
    expect(homepageItemVisibility(value, draft, hero)).toEqual(['disabled', 'disabled', 'disabled'])
  })

  it('现场先筛 scheduled 与严格未来时间，再按含重复的位置计 limit', () => {
    const value = snapshot()
    value.events.push(
      { ...value.events[0]!, id: 'event_0ad8f2a4-d0ed-4afb-9037-10cebec13752', dateTime: value.generatedAt },
      { ...value.events[0]!, id: 'event_ddc71348-db6d-4609-b17f-dce4b79bd768', status: 'sold-out' },
      { ...value.events[0]!, id: 'event_78633559-ba9a-4553-ade9-cd85e22fe4d0', status: 'cancelled' },
      { ...value.events[0]!, id: 'event_708ad0b2-00f9-4227-9f10-4a51108eaa58', dateTime: '2026-08-28T00:00:00Z' },
    )
    const draft = createHomepageDraft(value)
    const events = section(draft, 'event')
    events.itemIds = ['event_0ad8f2a4-d0ed-4afb-9037-10cebec13752', 'event_ddc71348-db6d-4609-b17f-dce4b79bd768', 'event_78633559-ba9a-4553-ade9-cd85e22fe4d0', 'event_708ad0b2-00f9-4227-9f10-4a51108eaa58', 'event_dc200c4c-abbb-476e-984d-405470f2dbea', 'event_dc200c4c-abbb-476e-984d-405470f2dbea', 'event_b9413813-c666-4987-bb0c-2850fd7fa156']
    events.limit = 2
    expect(homepageItemVisibility(value, draft, events)).toEqual(['time-window', 'time-window', 'time-window', 'time-window', 'visible', 'visible', 'limit'])
    const resolved = resolveHomepageSections({ ...value, homepage: { sections: draft.sections } }, 'zh-CN')
    expect(resolved.find(item => item.type === 'event')?.items.map(item => item.id)).toEqual(['event_dc200c4c-abbb-476e-984d-405470f2dbea', 'event_dc200c4c-abbb-476e-984d-405470f2dbea'])
    events.enabled = false
    expect(homepageItemVisibility(value, draft, events).every(state => state === 'disabled')).toBe(true)
  })

  it('尚未校验的日期、缺失记录和空引用不崩溃', () => {
    const value = snapshot()
    const draft = createHomepageDraft(value)
    const hero = section(draft, 'hero')
    draft.heroSlides[0]!.startsAt = 'incomplete'
    hero.itemIds = ['hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb', 'hero_2bc0af39-db42-40a8-a085-a589d34aac7d']
    expect(homepageItemVisibility(value, draft, hero)).toEqual(['time-window', 'time-window'])
    const events = section(draft, 'event')
    events.itemIds = ['event_859ea9cf-0be8-4afd-b5ff-9b94dc67823a']
    expect(homepageItemVisibility(value, draft, events)).toEqual(['time-window'])
    events.itemIds = []
    expect(homepageItemVisibility(value, draft, events)).toEqual([])
  })
})
