import type { Asset, YujianContentSnapshot } from '@yujian/schema'
import { describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import {
  AssetFileError,
  insertSnapshotAsset,
  sha256File,
  toSnapshotAsset,
  validateAssetFile,
} from '../../utils/asset-workbench'
import type { AdminAsset } from '../../utils/admin-api'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

const fixture = fixtureData as unknown as YujianContentSnapshot

describe('asset workbench utilities', () => {
  it('validates supported file rules without allocating large files', () => {
    expect(validateAssetFile({ name: 'cover.WEBP', size: 1024, type: 'image/webp' })).toMatchObject({
      kind: 'image',
      contentType: 'image/webp',
      maxBytes: 20 * 1024 * 1024,
    })
    expect(validateAssetFile({ name: 'preview.mp3', size: 1024, type: 'audio/mpeg' })).toMatchObject({
      kind: 'audio',
      maxBytes: 100 * 1024 * 1024,
    })
    expect(validateAssetFile({ name: 'video.mp4', size: 2 * 1024 * 1024 * 1024, type: 'video/mp4' })).toMatchObject({
      kind: 'video',
      maxBytes: 2 * 1024 * 1024 * 1024,
    })
  })

  it.each([
    [{ name: 'empty.webp', size: 0, type: 'image/webp' }, 'empty-file'],
    [{ name: 'cover.jpg', size: 1, type: 'image/webp' }, 'invalid-extension'],
    [{ name: 'cover.svg', size: 1, type: 'image/svg+xml' }, 'unsupported-type'],
    [{ name: 'cover.webp', size: 20 * 1024 * 1024 + 1, type: 'image/webp' }, 'file-too-large'],
  ] as const)('rejects invalid files with stable code %s', (file, code) => {
    expect(() => validateAssetFile(file)).toThrowError(AssetFileError)
    try {
      validateAssetFile(file)
    } catch (error) {
      expect(error).toMatchObject({ code })
    }
  })

  it('computes a lowercase SHA-256 checksum', async () => {
    await expect(sha256File(new Blob(['abc']))).resolves.toBe(
      'sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad',
    )
  })

  it('hashes large files incrementally without reading the whole Blob', async () => {
    const blob = new Blob([new Uint8Array(8 * 1024 * 1024 + 1)])
    const wholeBlobRead = vi.spyOn(blob, 'arrayBuffer').mockRejectedValue(new Error('whole Blob read'))
    const slice = vi.spyOn(blob, 'slice')

    await expect(sha256File(blob)).resolves.toMatch(/^sha256:[a-f0-9]{64}$/)

    expect(wholeBlobRead).not.toHaveBeenCalled()
    expect(slice).toHaveBeenCalledTimes(3)
  })

  it('converts ready server metadata into the canonical asset contract', () => {
    const asset = readyAsset()

    expect(toSnapshotAsset(asset, { zhCN: '封面', en: 'Cover' })).toEqual({
      id: 'asset_uploaded',
      kind: 'image',
      src: 'https://media.yujian.me/assets/asset_uploaded/source.webp',
      mimeType: 'image/webp',
      byteSize: 1024,
      width: 1200,
      height: 1200,
      durationSeconds: 3,
      alt: { 'zh-CN': '封面', en: 'Cover' },
      rights: { source: { 'zh-CN': '官方授权' }, credit: '王子健' },
      checksum: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    } satisfies Asset)
  })

  it('inserts a unique asset and keeps the canonical snapshot valid', () => {
    const asset = toSnapshotAsset(readyAsset(), { zhCN: '新素材' })
    const result = insertSnapshotAsset(JSON.stringify(fixture), asset)

    expect(result.error).toBeNull()
    expect(result.inserted).toBe(true)
    expect(result.text).toContain('asset_uploaded')
    expect(analyzeSnapshotText(result.text).issues).toHaveLength(0)

    expect(insertSnapshotAsset(result.text, asset)).toMatchObject({
      inserted: false,
      error: 'duplicate-id',
      text: result.text,
    })
  })

  it('refuses to insert into an invalid canonical snapshot', () => {
    expect(insertSnapshotAsset('{}', toSnapshotAsset(readyAsset(), { zhCN: '新素材' }))).toEqual({
      inserted: false,
      error: 'invalid-snapshot',
      text: '{}',
    })
  })
})

/** 构造具有完整服务端 metadata 的已确认素材。 */
function readyAsset(): AdminAsset {
  return {
    id: 'asset_uploaded',
    src: 'https://media.yujian.me/assets/asset_uploaded/source.webp',
    status: 'ready',
    metadata: {
      fileName: 'cover.webp',
      contentType: 'image/webp',
      declaredSize: 1024,
      checksum: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      width: 1200,
      height: 1200,
      duration: 3_000_000_000,
    },
    rights: { source: { 'zh-CN': '官方授权' }, credit: '王子健' },
    createdAt: '2026-10-09T08:00:00Z',
  }
}
