import type { Event, YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'
import { createEntityId } from './identity'

export interface EventDraft { events: Event[] }
export type EventErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-event' | 'invalid-draft'
export interface EventApplyResult { text: string; error: EventErrorCode | null; issues: readonly SnapshotEditorIssue[] }

/** 深复制所有现场，未应用输入不修改快照。 */
export function createEventDraft(snapshot: YujianContentSnapshot): EventDraft {
  return structuredClone({ events: snapshot.events })
}

/** 生成空记录供填写，不编造标题、素材或有效外链。 */
export function createEvent(): Event {
  return {
    id: createEntityId('event_'),
    title: { 'zh-CN': '' }, city: { 'zh-CN': '' }, venue: { 'zh-CN': '' },
    dateTime: '', status: 'scheduled', detailUrl: '',
  }
}

/** 基线一致且完整有效时只替换 events；拒绝删除或改名已有记录。 */
export function applyEventDraft(text: string, baseline: string, draft: EventDraft): EventApplyResult {
  /** 失败保留原始文本与可定位诊断。 */
  const fail = (error: EventErrorCode, issues: readonly SnapshotEditorIssue[] = []): EventApplyResult => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return fail('invalid-snapshot', analysis.issues)
  if (!draft || !Array.isArray(draft.events)
    || draft.events.some(event => !event || typeof event !== 'object' || Array.isArray(event))) return fail('invalid-draft')
  const snapshot = analysis.snapshot
  const ids = new Set(draft.events.map(event => event.id))
  if (snapshot.events.some(event => !ids.has(event.id))) return fail('immutable-event')
  let nextText: string
  try {
    nextText = JSON.stringify({ ...snapshot, events: draft.events }, null, 2)
  } catch {
    return fail('invalid-draft')
  }
  const next = analyzeSnapshotText(nextText)
  return next.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', next.issues)
}
