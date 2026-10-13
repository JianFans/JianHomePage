import type { HeroSlide, LocalizedText, YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export interface HomepageDraft {
  heroSlides: HeroSlide[]
  sections: YujianContentSnapshot['homepage']['sections']
}

export type HomepageErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-section' | 'invalid-draft'
export type HomepageItemVisibility = 'visible' | 'disabled' | 'limit' | 'time-window'
export type HomepageSection = HomepageDraft['sections'][number]

export interface HomepageApplyResult {
  text: string
  error: HomepageErrorCode | null
  issues: readonly SnapshotEditorIssue[]
}

export interface HomepageItemOption {
  id: string
  title: LocalizedText
}

/** 深复制可编辑的首页字段，使未应用输入不会修改当前快照。 */
export function createHomepageDraft(snapshot: YujianContentSnapshot): HomepageDraft {
  return structuredClone({ heroSlides: snapshot.heroSlides, sections: snapshot.homepage.sections })
}

/** 生成空宣传位供填写；不编造标题或素材，也不自动加入板块。 */
export function createHeroSlide(): HeroSlide {
  return {
    id: `hero_${crypto.randomUUID().replaceAll('-', '_')}`,
    mediaKind: 'image', assetId: '', headline: { 'zh-CN': '' },
    focalPoint: { x: 50, y: 50 }, autoplay: false,
  }
}

/** 删除临时记录及所有宣传位板块中的对应位置，其他内容目标留给完整校验。 */
export function removeHomepageHero(draft: HomepageDraft, id: string): void {
  draft.heroSlides = draft.heroSlides.filter(slide => slide.id !== id)
  for (const section of draft.sections) {
    if (section.type === 'hero') {
      section.itemIds = section.itemIds.filter(itemId => itemId !== id) as typeof section.itemIds
    }
  }
}

/** 用结构化三元组标识不可变字段，避免同 ID 的不同类型或布局互相覆盖。 */
function sectionIdentity(section: HomepageSection): string {
  return JSON.stringify([section.id, section.type, section.layoutVariant])
}

/**
 * 在未变化的有效基线上原子替换首页字段；板块身份与布局不可修改。
 * 完整 canonical 诊断保证素材及内部目标仍有效，失败时返回输入原文。
 */
export function applyHomepageDraft(text: string, baseline: string, draft: HomepageDraft): HomepageApplyResult {
  /** 返回稳定错误代码及共享诊断，不改写调用方的输入。 */
  const fail = (error: HomepageErrorCode, issues: readonly SnapshotEditorIssue[] = []): HomepageApplyResult => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return fail('invalid-snapshot', analysis.issues)
  if (!draft || !Array.isArray(draft.heroSlides) || !Array.isArray(draft.sections)
    || draft.sections.some(section => !section || typeof section !== 'object')
    || draft.heroSlides.some(slide => !slide || typeof slide !== 'object')) return fail('invalid-draft')

  const snapshot = analysis.snapshot
  if (draft.sections.length !== snapshot.homepage.sections.length) return fail('immutable-section')
  // canonical 允许板块重复；按身份计数而非按 ID 去重，保留每一个既有板块。
  const identities = new Map<string, number>()
  for (const section of snapshot.homepage.sections) {
    const key = sectionIdentity(section)
    identities.set(key, (identities.get(key) ?? 0) + 1)
  }
  for (const section of draft.sections) {
    if (typeof section.id !== 'string' || typeof section.type !== 'string'
      || typeof section.layoutVariant !== 'string') return fail('immutable-section')
    const key = sectionIdentity(section)
    const remaining = identities.get(key) ?? 0
    if (remaining === 0) return fail('immutable-section')
    identities.set(key, remaining - 1)
  }

  let nextText: string
  try {
    nextText = JSON.stringify({
      ...snapshot,
      heroSlides: draft.heroSlides,
      homepage: { ...snapshot.homepage, sections: draft.sections },
    }, null, 2)
  } catch {
    return fail('invalid-draft')
  }
  const nextAnalysis = analyzeSnapshotText(nextText)
  return nextAnalysis.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', nextAnalysis.issues)
}

/** 为当前板块提供已有内容，宣传位使用草稿标题；片段无文案时使用稳定 ID。 */
export function homepageItemOptions(
  snapshot: YujianContentSnapshot,
  draft: HomepageDraft,
  section: HomepageSection,
): HomepageItemOption[] {
  switch (section.type) {
    case 'hero': return (Array.isArray(draft.heroSlides) ? draft.heroSlides : [])
      .filter(slide => Boolean(slide))
      .map(slide => ({ id: slide.id, title: slide.headline ?? { 'zh-CN': '' } }))
    case 'music': return snapshot.releases.map(item => ({ id: item.id, title: item.title }))
    case 'video': return snapshot.videos.map(item => ({ id: item.id, title: item.title }))
    case 'event': return snapshot.events.map(item => ({ id: item.id, title: item.title }))
    case 'moment': return snapshot.moments.map(item => ({ id: item.id, title: item.caption ?? { 'zh-CN': item.id } }))
    case 'artist': return [{ id: snapshot.artist.id, title: snapshot.artist.name }]
    default: return []
  }
}

/**
 * 按 generatedAt 标记每个引用位置，重复引用各占一个位置。
 * 停用优先；现场先排除时间或状态不符的记录，再计上限，未完成日期安全地视为窗口外。
 */
export function homepageItemVisibility(
  snapshot: YujianContentSnapshot,
  draft: HomepageDraft,
  section: HomepageSection,
): HomepageItemVisibility[] {
  if (!Array.isArray(section.itemIds)) return []
  if (!section.enabled) return section.itemIds.map(() => 'disabled')
  const referenceTime = Date.parse(snapshot.generatedAt)
  if (section.type === 'hero') {
    const heroes = new Map((Array.isArray(draft.heroSlides) ? draft.heroSlides : [])
      .filter(slide => Boolean(slide)).map(slide => [slide.id, slide]))
    return section.itemIds.map((id) => {
      const slide = heroes.get(id)
      if (!slide) return 'time-window'
      const startsAt = slide.startsAt ? Date.parse(slide.startsAt) : Number.NEGATIVE_INFINITY
      const endsAt = slide.endsAt ? Date.parse(slide.endsAt) : Number.POSITIVE_INFINITY
      return startsAt <= referenceTime && referenceTime < endsAt ? 'visible' : 'time-window'
    })
  }
  if (section.type === 'event') {
    const events = new Map(snapshot.events.map(event => [event.id, event]))
    let eligible = 0
    return section.itemIds.map((id) => {
      const event = events.get(id)
      if (!event || event.status !== 'scheduled' || !(Date.parse(event.dateTime) > referenceTime)) return 'time-window'
      return eligible++ < section.limit ? 'visible' : 'limit'
    })
  }
  if ('limit' in section) return section.itemIds.map((_, index) => index < section.limit ? 'visible' : 'limit')
  return section.itemIds.map(() => 'visible')
}
