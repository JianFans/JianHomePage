import type { YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export type SiteDraft = Pick<YujianContentSnapshot, 'site' | 'artist'>
export type SiteError = 'stale-snapshot' | 'invalid-snapshot' | 'invalid-draft' | 'immutable-settings'

/** 资料属于内容版本，独立草稿不会直接改变已保存快照。 */
export function createSiteDraft(snapshot: YujianContentSnapshot): SiteDraft {
  return structuredClone({ site: snapshot.site, artist: snapshot.artist })
}

/** 原子替换资料，保留语言、未来导航及艺人身份，失败返回原文。 */
export function applySiteDraft(text: string, baseline: string, draft: SiteDraft): { text: string; error: SiteError | null; issues: readonly SnapshotEditorIssue[] } {
  const fail = (error: SiteError, issues: readonly SnapshotEditorIssue[] = []) => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const before = analyzeSnapshotText(text)
  if (!before.snapshot) return fail('invalid-snapshot', before.issues)
  if (!draft?.site || !draft.artist) return fail('invalid-draft')
  const { site, artist } = before.snapshot
  if (draft.artist.id !== artist.id || draft.site.defaultLocale !== site.defaultLocale
    || JSON.stringify(draft.site.supportedLocales) !== JSON.stringify(site.supportedLocales)
    || JSON.stringify(draft.site.futureNavigation) !== JSON.stringify(site.futureNavigation)) return fail('immutable-settings')
  const nextText = JSON.stringify({ ...before.snapshot, site: draft.site, artist: draft.artist }, null, 2)
  const next = analyzeSnapshotText(nextText)
  return next.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', next.issues)
}
