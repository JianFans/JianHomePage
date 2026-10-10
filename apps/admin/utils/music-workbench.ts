import type { Asset, Release, Track, YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export interface MusicDraft {
  originalId: string | null
  release: Omit<Release, 'trackIds'>
  tracks: Track[]
  sections: { id: string; itemIds: string[] }[]
}

export type MusicErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'missing-release' | 'immutable-id'
  | 'duplicate-id' | 'track-ownership' | 'invalid-draft'

export interface MusicApplyResult {
  text: string
  error: MusicErrorCode | null
  issues: readonly SnapshotEditorIssue[]
}

/** 生成带类型前缀的随机 ID；应用事务仍会检查与快照记录的冲突。 */
function musicId(prefix: 'release' | 'track'): string {
  return `${prefix}_${crypto.randomUUID().replaceAll('-', '_')}`
}

/** 创建未填写的曲目副本，不为标题、时长或平台链接编造内容。 */
export function createMusicTrack(releaseId: string): Track {
  return { id: musicId('track'), releaseId, title: { 'zh-CN': '' }, durationSeconds: 0, platformLinks: [], credits: [] }
}

/** 从有效快照复制作品与有序曲目；新作品保留空必填字段供编辑人员填写。 */
export function createMusicDraft(snapshot: YujianContentSnapshot, releaseId: string | null): MusicDraft | null {
  const original = snapshot.releases.find(release => release.id === releaseId)
  if (releaseId !== null && !original) return null
  const release: Omit<Release, 'trackIds'> = original || {
    id: musicId('release'), kind: 'single', title: { 'zh-CN': '' }, coverAssetId: '', releaseDate: '',
    platformLinks: [], featured: false,
  }
  const fields: Omit<Release, 'trackIds'> = {
    id: release.id, kind: release.kind, title: release.title, coverAssetId: release.coverAssetId,
    releaseDate: release.releaseDate, platformLinks: release.platformLinks, featured: release.featured,
  }
  return structuredClone({
    originalId: releaseId,
    release: fields,
    tracks: original
      ? original.trackIds.flatMap(id => snapshot.tracks.find(track => track.id === id) ?? [])
      : [createMusicTrack(release.id)],
    sections: snapshot.homepage.sections.filter(section => section.type === 'music')
      .map(section => ({ id: section.id, itemIds: section.itemIds })),
  })
}

/** 只从当前快照提供符合用途的素材，不查询或自动插入服务端素材。 */
export function musicAssets(snapshot: YujianContentSnapshot, purpose: 'cover' | 'preview'): Asset[] {
  return snapshot.assets.filter(asset => purpose === 'cover' ? ['image', 'gif'].includes(asset.kind) : asset.kind === 'audio')
}

/**
 * 在最新文本上原子应用作品、曲目与音乐条目；任何错误都返回未经修改的原文。
 * 基线比较防止表单覆盖其他入口的编辑，完整诊断保护素材引用和首页内部目标。
 */
export function applyMusicDraft(text: string, baseline: string, draft: MusicDraft): MusicApplyResult {
  /** 保留输入原文，并返回可本地化代码和可定位的共享诊断。 */
  const fail = (error: MusicErrorCode, issues: readonly SnapshotEditorIssue[] = []): MusicApplyResult => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return fail('invalid-snapshot', analysis.issues)
  const snapshot = analysis.snapshot
  const original = snapshot.releases.find(release => release.id === draft.originalId)
  if (draft.originalId !== null && !original) return fail('missing-release')
  if (original && original.id !== draft.release.id) return fail('immutable-id')
  if (!original && snapshot.releases.some(release => release.id === draft.release.id)) return fail('duplicate-id')
  const trackIds = draft.tracks.map(track => track.id)
  // trackIds 可重复引用同一记录，但不能让同一 ID 的不同字段互相覆盖。
  const updatedTracks = new Map<string, Track>()
  for (const track of draft.tracks) {
    const previous = updatedTracks.get(track.id)
    if (previous && previous !== track && JSON.stringify(previous) !== JSON.stringify(track)) return fail('duplicate-id')
    updatedTracks.set(track.id, track)
  }
  const previousIds = new Set(original?.trackIds ?? [])
  const existingTracks = new Map(snapshot.tracks.map(track => [track.id, track]))
  if (draft.tracks.some(track => track.releaseId !== draft.release.id
    || (existingTracks.has(track.id) && !previousIds.has(track.id)))) return fail('track-ownership')

  const musicSections = snapshot.homepage.sections.filter(section => section.type === 'music')
  const sectionChanges = new Map(draft.sections.map(section => [section.id, section.itemIds]))
  if (sectionChanges.size !== draft.sections.length || sectionChanges.size !== musicSections.length
    || musicSections.some(section => !sectionChanges.has(section.id))) return fail('invalid-draft')

  const release: Release = { ...draft.release, trackIds: trackIds as Release['trackIds'] }
  const next: YujianContentSnapshot = {
    ...snapshot,
    releases: original
      ? snapshot.releases.map(item => item.id === original.id ? release : item)
      : [...snapshot.releases, release],
    tracks: [
      ...snapshot.tracks.filter(track => !previousIds.has(track.id) || updatedTracks.has(track.id))
        .map(track => updatedTracks.get(track.id) ?? track),
      ...[...updatedTracks.values()].filter(track => !existingTracks.has(track.id)),
    ],
    homepage: {
      ...snapshot.homepage,
      sections: snapshot.homepage.sections.map(section => section.type === 'music'
        ? { ...section, itemIds: sectionChanges.get(section.id)! }
        : section) as YujianContentSnapshot['homepage']['sections'],
    },
  }
  const nextText = JSON.stringify(next, null, 2)
  const nextAnalysis = analyzeSnapshotText(nextText)
  return nextAnalysis.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', nextAnalysis.issues)
}
