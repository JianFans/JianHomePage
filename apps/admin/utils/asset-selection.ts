import type { Asset } from '@yujian/schema'
import { analyzeSnapshotText } from './snapshot-workbench'
import { equalSnapshotValue } from './snapshot-diff'
import type { InjectionKey } from 'vue'

export type AssetPickerRequest = { kinds: readonly Asset['kind'][]; label: string }
export type PickAsset = (request: AssetPickerRequest) => Promise<Asset | null>
export const assetPickerKey: InjectionKey<PickAsset> = Symbol('studio-asset-picker')

/** 素材选择只允许追加，不覆盖同 ID 的已发布元数据。 */
export function appendPickedAsset(text: string, asset: Asset, kinds: readonly Asset['kind'][]): { text: string; error: 'invalid-snapshot' | 'invalid-asset' | 'conflict' | null } {
  const snapshot = analyzeSnapshotText(text).snapshot
  if (!snapshot) return { text, error: 'invalid-snapshot' }
  if (!kinds.includes(asset.kind)) return { text, error: 'invalid-asset' }
  const existing = snapshot.assets.find(item => item.id === asset.id)
  if (existing) return { text, error: equalSnapshotValue(existing, asset) ? null : 'conflict' }
  const next = JSON.stringify({ ...snapshot, assets: [...snapshot.assets, asset] }, null, 2)
  return analyzeSnapshotText(next).snapshot ? { text: next, error: null } : { text, error: 'invalid-asset' }
}

/** 仅追加且所有旧素材及其他字段不变时允许重建编辑基线。 */
export function isAssetAppendOnly(before: string, after: string): boolean {
  const previous = analyzeSnapshotText(before).snapshot
  const next = analyzeSnapshotText(after).snapshot
  if (!previous || !next || next.assets.length <= previous.assets.length) return false
  return JSON.stringify(previous.assets) === JSON.stringify(next.assets.slice(0, previous.assets.length))
    && JSON.stringify({ ...previous, assets: [] }) === JSON.stringify({ ...next, assets: [] })
}
