import type { YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export type ContentDeletionKind = 'release' | 'track' | 'video' | 'event' | 'moment' | 'hero'
export type ContentDeletionErrorCode = 'invalid-snapshot' | 'missing-target' | 'last-track' | 'blocked-reference' | 'stale-snapshot' | 'invalid-result'
export interface ContentDeletionReference { path: string; contentId: string; action: 'arrangement' | 'ownership' | 'cascade' | 'blocking' }
export interface ContentDeletionRecord { kind: ContentDeletionKind; id: string; path: string }
export interface ContentDeletionPlan {
  baseline: string
  kind: ContentDeletionKind
  id: string
  removed: ContentDeletionRecord[]
  references: ContentDeletionReference[]
  error: ContentDeletionErrorCode | null
  issues: readonly SnapshotEditorIssue[]
}
export interface ContentDeletionResult { text: string; error: ContentDeletionErrorCode | null; issues: readonly SnapshotEditorIssue[] }

const collections = { release: 'releases', track: 'tracks', video: 'videos', event: 'events', moment: 'moments', hero: 'heroSlides' } as const

/** 只分析 canonical 快照；引用位置保留原始数组索引，供明确确认。 */
export function analyzeContentDeletion(text: string, kind: ContentDeletionKind, id: string): ContentDeletionPlan {
  const plan: ContentDeletionPlan = { baseline: text, kind, id, removed: [], references: [], error: null, issues: [] }
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return { ...plan, error: 'invalid-snapshot', issues: analysis.issues }
  const snapshot = analysis.snapshot
  const key = collections[kind]
  if (!key) return { ...plan, error: 'missing-target' }
  const index = snapshot[key].findIndex(record => record.id === id)
  if (index < 0) return { ...plan, error: 'missing-target' }
  plan.removed.push({ kind, id, path: `/${key}/${index}` })
  if (kind === 'release') {
    snapshot.tracks.forEach((track, trackIndex) => {
      if (track.releaseId === id) plan.removed.push({ kind: 'track', id: track.id, path: `/tracks/${trackIndex}` })
    })
  }
  const removedIds = new Set(plan.removed.map(record => record.id))
  const reference = (contentId: string, path: string, action: ContentDeletionReference['action']) => {
    if (!removedIds.has(contentId)) return
    const cascades = plan.removed.some(record => path.startsWith(`${record.path}/`))
    plan.references.push({ path, contentId, action: cascades ? 'cascade' : action })
  }
  snapshot.homepage.sections.forEach((section, sectionIndex) => {
    section.itemIds.forEach((itemId, itemIndex) => reference(itemId, `/homepage/sections/${sectionIndex}/itemIds/${itemIndex}`, 'arrangement'))
  })
  snapshot.heroSlides.forEach((hero, heroIndex) => {
    if (hero.releaseId) reference(hero.releaseId, `/heroSlides/${heroIndex}/releaseId`, 'blocking')
    if (hero.target?.kind === 'internal') reference(hero.target.contentId, `/heroSlides/${heroIndex}/target/contentId`, 'blocking')
  })
  snapshot.moments.forEach((moment, momentIndex) => {
    if (moment.target?.kind === 'internal') reference(moment.target.contentId, `/moments/${momentIndex}/target/contentId`, 'blocking')
  })
  snapshot.releases.forEach((release, releaseIndex) => {
    release.trackIds.forEach((itemId, itemIndex) => reference(itemId, `/releases/${releaseIndex}/trackIds/${itemIndex}`, 'ownership'))
  })
  snapshot.tracks.forEach((track, trackIndex) => reference(track.releaseId, `/tracks/${trackIndex}/releaseId`, 'blocking'))
  if (kind === 'track' && snapshot.releases.some(release => release.trackIds.includes(id) && !release.trackIds.some(track => track !== id))) {
    plan.error = 'last-track'
  } else if (plan.references.some(item => item.action === 'blocking')) plan.error = 'blocked-reference'
  return plan
}

/** 精确基线与重新分析保护确认；最终校验失败时保留调用者的原文。 */
export function applyContentDeletion(text: string, plan: ContentDeletionPlan): ContentDeletionResult {
  const fail = (error: ContentDeletionErrorCode, issues: readonly SnapshotEditorIssue[] = []): ContentDeletionResult => ({ text, error, issues })
  if (text !== plan.baseline) return fail('stale-snapshot')
  const current = analyzeContentDeletion(text, plan.kind, plan.id)
  if (current.error) return fail(current.error, current.issues)
  const snapshot = analyzeSnapshotText(text).snapshot!
  const ids = new Set(current.removed.map(record => record.id))
  const next: YujianContentSnapshot = {
    ...snapshot,
    releases: snapshot.releases.filter(record => !ids.has(record.id)).map(release => ({
      ...release, trackIds: release.trackIds.filter(id => !ids.has(id)) as typeof release.trackIds,
    })),
    tracks: snapshot.tracks.filter(record => !ids.has(record.id)),
    videos: snapshot.videos.filter(record => !ids.has(record.id)),
    events: snapshot.events.filter(record => !ids.has(record.id)),
    moments: snapshot.moments.filter(record => !ids.has(record.id)),
    heroSlides: snapshot.heroSlides.filter(record => !ids.has(record.id)),
    homepage: { sections: snapshot.homepage.sections.map(section => ({
      ...section, itemIds: section.itemIds.filter(id => !ids.has(id)),
    })) as YujianContentSnapshot['homepage']['sections'] },
  }
  const nextText = JSON.stringify(next, null, 2)
  const analysis = analyzeSnapshotText(nextText)
  return analysis.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-result', analysis.issues)
}
