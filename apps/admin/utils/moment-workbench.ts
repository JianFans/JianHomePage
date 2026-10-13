import type { Moment, YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export interface MomentDraft { moments: Moment[] }
export type MomentErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-moment' | 'invalid-draft'
export interface MomentApplyResult { text: string; error: MomentErrorCode | null; issues: readonly SnapshotEditorIssue[] }

/** 深复制所有片段，未应用输入不修改快照。 */
export function createMomentDraft(snapshot: YujianContentSnapshot): MomentDraft {
  return structuredClone({ moments: snapshot.moments })
}

/** 生成空记录供填写，不编造标题、素材或有效外链。 */
export function createMoment(): Moment {
  return {
    id: `moment_${crypto.randomUUID().replaceAll('-', '_')}`,
    assetId: '',
  }
}

/** 基线一致且完整有效时只替换 moments；拒绝删除或改名已有记录。 */
export function applyMomentDraft(text: string, baseline: string, draft: MomentDraft): MomentApplyResult {
  /** 失败保留原始文本与可定位诊断。 */
  const fail = (error: MomentErrorCode, issues: readonly SnapshotEditorIssue[] = []): MomentApplyResult => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return fail('invalid-snapshot', analysis.issues)
  if (!draft || !Array.isArray(draft.moments)
    || draft.moments.some(moment => !moment || typeof moment !== 'object' || Array.isArray(moment))) return fail('invalid-draft')
  const snapshot = analysis.snapshot
  const ids = new Set(draft.moments.map(moment => moment.id))
  if (snapshot.moments.some(moment => !ids.has(moment.id))) return fail('immutable-moment')
  let nextText: string
  try {
    nextText = JSON.stringify({ ...snapshot, moments: draft.moments }, null, 2)
  } catch {
    return fail('invalid-draft')
  }
  const next = analyzeSnapshotText(nextText)
  return next.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', next.issues)
}
