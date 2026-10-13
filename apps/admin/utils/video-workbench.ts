import type { Video, YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from './snapshot-workbench'

export interface VideoDraft { videos: Video[] }
export type VideoErrorCode = 'stale-snapshot' | 'invalid-snapshot' | 'immutable-video' | 'invalid-draft'
export interface VideoApplyResult { text: string; error: VideoErrorCode | null; issues: readonly SnapshotEditorIssue[] }

/** 深复制所有影像，未应用输入不修改快照。 */
export function createVideoDraft(snapshot: YujianContentSnapshot): VideoDraft {
  return structuredClone({ videos: snapshot.videos })
}

/** 生成空记录供填写，不编造标题、素材或有效外链。 */
export function createVideo(): Video {
  return {
    id: `video_${crypto.randomUUID().replaceAll('-', '_')}`,
    title: { 'zh-CN': '' }, posterAssetId: '', durationSeconds: 1, featured: false,
    platformLinks: [{ provider: 'bilibili', url: '' }],
  }
}

/** 基线一致且完整有效时只替换 videos；拒绝删除或改名已有记录。 */
export function applyVideoDraft(text: string, baseline: string, draft: VideoDraft): VideoApplyResult {
  /** 失败保留原始文本与可定位诊断。 */
  const fail = (error: VideoErrorCode, issues: readonly SnapshotEditorIssue[] = []): VideoApplyResult => ({ text, error, issues })
  if (text !== baseline) return fail('stale-snapshot')
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return fail('invalid-snapshot', analysis.issues)
  if (!draft || !Array.isArray(draft.videos)
    || draft.videos.some(video => !video || typeof video !== 'object' || Array.isArray(video))) return fail('invalid-draft')
  const snapshot = analysis.snapshot
  const ids = new Set(draft.videos.map(video => video.id))
  if (snapshot.videos.some(video => !ids.has(video.id))) return fail('immutable-video')
  let nextText: string
  try {
    nextText = JSON.stringify({ ...snapshot, videos: draft.videos }, null, 2)
  } catch {
    return fail('invalid-draft')
  }
  const next = analyzeSnapshotText(nextText)
  return next.snapshot ? { text: nextText, error: null, issues: [] } : fail('invalid-draft', next.issues)
}
