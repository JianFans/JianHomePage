import {
  diagnoseContentSnapshot,
  type ContentSnapshotIssueSource,
  type YujianContentSnapshot,
} from '@yujian/schema'

export const MAX_SNAPSHOT_IMPORT_BYTES = 2 * 1024 * 1024

export interface SnapshotEditorIssue {
  path: string
  source: 'editor' | ContentSnapshotIssueSource
  code: string
}

export interface SnapshotSummary {
  sections: number
  releases: number
  tracks: number
  videos: number
  events: number
  moments: number
  assets: number
  previews: number
}

export interface SnapshotAnalysis {
  snapshot: YujianContentSnapshot | null
  issues: readonly SnapshotEditorIssue[]
  summary: SnapshotSummary | null
}

export interface SnapshotImportFile {
  name: string
  size: number
  text(): Promise<string>
}

export interface SnapshotExport {
  filename: string
  contents: string
  mimeType: 'application/json'
}

export type SnapshotImportErrorCode =
  | 'invalid-extension'
  | 'empty-file'
  | 'file-too-large'
  | 'read-failed'

export class SnapshotImportError extends Error {
  readonly code: SnapshotImportErrorCode

  /** 使用稳定错误代码描述可本地化的导入失败。 */
  constructor(code: SnapshotImportErrorCode) {
    super(code)
    this.name = 'SnapshotImportError'
    this.code = code
  }
}

/**
 * 解析并校验编辑器文本，只为完全有效的快照生成摘要。
 */
export function analyzeSnapshotText(text: string): SnapshotAnalysis {
  let value: unknown
  try {
    value = JSON.parse(text)
  } catch {
    return invalidAnalysis('invalid-json')
  }

  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    return invalidAnalysis('object-root')
  }

  const issues = diagnoseContentSnapshot(value)
  if (issues.length > 0) {
    return { snapshot: null, issues, summary: null }
  }

  const snapshot = value as YujianContentSnapshot
  return {
    snapshot,
    issues: [],
    summary: summarizeSnapshot(snapshot),
  }
}

/**
 * 校验导入文件的扩展名、大小和可读性，并返回原始 JSON 文本。
 */
export async function readSnapshotImport(file: SnapshotImportFile): Promise<string> {
  if (!file.name.toLowerCase().endsWith('.json')) {
    throw new SnapshotImportError('invalid-extension')
  }
  if (file.size <= 0) {
    throw new SnapshotImportError('empty-file')
  }
  if (file.size > MAX_SNAPSHOT_IMPORT_BYTES) {
    throw new SnapshotImportError('file-too-large')
  }

  let contents: string
  try {
    contents = await file.text()
  } catch {
    throw new SnapshotImportError('read-failed')
  }
  if (!contents.trim()) {
    throw new SnapshotImportError('empty-file')
  }
  return contents
}

/**
 * 将已验证快照序列化为具有稳定文件名和末尾换行的 JSON 文件。
 */
export function createSnapshotExport(snapshot: YujianContentSnapshot): SnapshotExport {
  const filename = /^rel_[a-z0-9_]+$/.test(snapshot.releaseId)
    ? `${snapshot.releaseId}.json`
    : 'yujian-snapshot.json'
  return {
    filename,
    contents: `${JSON.stringify(snapshot, null, 2)}\n`,
    mimeType: 'application/json',
  }
}

/** 为编辑器级解析失败构造不含摘要的分析结果。 */
function invalidAnalysis(code: string): SnapshotAnalysis {
  return {
    snapshot: null,
    issues: [{ path: '/', source: 'editor', code }],
    summary: null,
  }
}

/** 汇总首页编辑器预览所需的内容与素材数量。 */
function summarizeSnapshot(snapshot: YujianContentSnapshot): SnapshotSummary {
  return {
    sections: snapshot.homepage.sections.length,
    releases: snapshot.releases.length,
    tracks: snapshot.tracks.length,
    videos: snapshot.videos.length,
    events: snapshot.events.length,
    moments: snapshot.moments.length,
    assets: snapshot.assets.length,
    previews: snapshot.tracks.filter(track => Boolean(track.previewAssetId)).length,
  }
}
