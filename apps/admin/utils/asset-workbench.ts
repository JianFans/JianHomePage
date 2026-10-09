import type { Asset } from '@yujian/schema'
import { sha256 } from '@noble/hashes/sha2.js'
import type { AdminAsset } from './admin-api'
import { analyzeSnapshotText } from './snapshot-workbench'

export type AssetFileErrorCode = 'empty-file' | 'unsupported-type' | 'invalid-extension' | 'file-too-large'
export type SnapshotInsertErrorCode = 'invalid-snapshot' | 'duplicate-id' | 'invalid-asset'

export interface AssetFileRule {
  kind: Asset['kind']
  contentType: Asset['mimeType']
  extension: string
  maxBytes: number
}

export interface LocalizedDraft {
  zhCN: string
  en?: string
}

export interface SnapshotInsertResult {
  text: string
  inserted: boolean
  error: SnapshotInsertErrorCode | null
}

/** 使用稳定错误代码表示无需发起网络请求的文件预检失败。 */
export class AssetFileError extends Error {
  readonly code: AssetFileErrorCode

  constructor(code: AssetFileErrorCode) {
    super(code)
    this.name = 'AssetFileError'
    this.code = code
  }
}

const fileRules: Readonly<Record<string, AssetFileRule>> = {
  'image/webp': { kind: 'image', contentType: 'image/webp', extension: '.webp', maxBytes: 20 * 1024 * 1024 },
  'image/gif': { kind: 'gif', contentType: 'image/gif', extension: '.gif', maxBytes: 20 * 1024 * 1024 },
  'audio/mpeg': { kind: 'audio', contentType: 'audio/mpeg', extension: '.mp3', maxBytes: 100 * 1024 * 1024 },
  'audio/wav': { kind: 'audio', contentType: 'audio/wav', extension: '.wav', maxBytes: 100 * 1024 * 1024 },
  'video/mp4': { kind: 'video', contentType: 'video/mp4', extension: '.mp4', maxBytes: 2 * 1024 * 1024 * 1024 },
}

const digestChunkBytes = 4 * 1024 * 1024

/** 在摘要计算前校验文件 MIME、扩展名和大小，并返回对应 canonical 规则。 */
export function validateAssetFile(file: Pick<File, 'name' | 'size' | 'type'>): AssetFileRule {
  if (!Number.isSafeInteger(file.size) || file.size <= 0) throw new AssetFileError('empty-file')
  const rule = fileRules[file.type]
  if (!rule) throw new AssetFileError('unsupported-type')
  const separator = file.name.lastIndexOf('.')
  const extension = separator >= 0 ? file.name.slice(separator).toLowerCase() : ''
  if (extension !== rule.extension) throw new AssetFileError('invalid-extension')
  if (file.size > rule.maxBytes) throw new AssetFileError('file-too-large')
  return rule
}

/** 分块计算可直接提交给服务端的标准 SHA-256，避免大文件整块进入内存。 */
export async function sha256File(file: Blob): Promise<string> {
  const hash = sha256.create()
  for (let offset = 0; offset < file.size; offset += digestChunkBytes) {
    const chunk = file.slice(offset, Math.min(offset + digestChunkBytes, file.size))
    hash.update(new Uint8Array(await chunk.arrayBuffer()))
  }
  const hex = Array.from(hash.digest(), byte => byte.toString(16).padStart(2, '0')).join('')
  return `sha256:${hex}`
}

/** 把服务端已确认素材转换为 canonical 快照素材，并把纳秒时长换算为秒。 */
export function toSnapshotAsset(asset: AdminAsset, alt: LocalizedDraft): Asset {
  if (asset.status !== 'ready') throw new Error('asset-not-ready')
  const rule = asset.metadata.contentType ? fileRules[asset.metadata.contentType] : undefined
  if (!rule || !Number.isSafeInteger(asset.metadata.declaredSize) || Number(asset.metadata.declaredSize) <= 0) {
    throw new Error('invalid-asset-metadata')
  }
  if (!/^sha256:[a-f0-9]{64}$/.test(asset.metadata.checksum || '') || !asset.src) {
    throw new Error('invalid-asset-metadata')
  }
  const zhCN = alt.zhCN.trim()
  if (!zhCN) throw new Error('missing-asset-alt')

  const result = {
    id: asset.id,
    kind: rule.kind,
    src: asset.src as Asset['src'],
    mimeType: rule.contentType,
    byteSize: Number(asset.metadata.declaredSize),
    alt: { 'zh-CN': zhCN },
    rights: {
      source: {
        'zh-CN': asset.rights.source['zh-CN'],
        ...(asset.rights.source.en ? { en: asset.rights.source.en } : {}),
      },
      ...(asset.rights.credit ? { credit: asset.rights.credit } : {}),
      ...(asset.rights.license ? { license: asset.rights.license } : {}),
    },
    checksum: asset.metadata.checksum!,
  } as Asset
  if (alt.en?.trim()) result.alt.en = alt.en.trim()
  if (positiveInteger(asset.metadata.width)) result.width = asset.metadata.width
  if (positiveInteger(asset.metadata.height)) result.height = asset.metadata.height
  if (typeof asset.metadata.duration === 'number' && Number.isFinite(asset.metadata.duration) && asset.metadata.duration > 0) {
    result.durationSeconds = asset.metadata.duration / 1_000_000_000
  }
  return result
}

/** 向当前合法快照追加素材；重复 ID 或追加后失效时保持原文本不变。 */
export function insertSnapshotAsset(text: string, asset: Asset): SnapshotInsertResult {
  const analysis = analyzeSnapshotText(text)
  if (!analysis.snapshot) return { text, inserted: false, error: 'invalid-snapshot' }
  if (analysis.snapshot.assets.some(item => item.id === asset.id)) {
    return { text, inserted: false, error: 'duplicate-id' }
  }
  const next = {
    ...analysis.snapshot,
    assets: [...analysis.snapshot.assets, structuredClone(asset)],
  }
  const nextText = JSON.stringify(next, null, 2)
  if (!analyzeSnapshotText(nextText).snapshot) {
    return { text, inserted: false, error: 'invalid-asset' }
  }
  return { text: nextText, inserted: true, error: null }
}

/** 判断未知 metadata 尺寸是否为 canonical 正整数。 */
function positiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0
}
