import { effectScope, ref, watch } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useAssetWorkspace, type AssetWorkspaceApi } from '../../composables/useAssetWorkspace'
import type { AdminAsset, AdminAssetUpload } from '../../utils/admin-api'

describe('asset workspace state machine', () => {
  it('runs the successful upload stages in order and adds the ready asset', async () => {
    const stages: string[] = []
    const api = createApi()
    const workspace = useAssetWorkspace({
      apiBaseUrl: ref('https://api.yujian.me'),
      token: ref('session-token'),
      apiFactory: () => api,
      digest: async () => 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    })
    watch(workspace.stage, value => stages.push(value), { flush: 'sync' })
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '官方授权'
    workspace.altZhCN.value = '封面'

    await workspace.upload()

    expect(stages).toEqual(['hashing', 'creating', 'uploading', 'completing', 'succeeded'])
    expect(api.createAssetUpload).toHaveBeenCalledWith(expect.objectContaining({
      fileName: 'cover.webp',
      contentType: 'image/webp',
      rights: { source: { 'zh-CN': '官方授权' } },
    }))
    expect(api.uploadAssetBlob).toHaveBeenCalledTimes(1)
    expect(api.completeAssetUpload).toHaveBeenCalledWith('asset_uploaded')
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_uploaded'])
    expect(workspace.file.value).toBeNull()
  })

  it('retains the form after blob upload failure so a new signature can be requested', async () => {
    const api = createApi()
    api.uploadAssetBlob.mockRejectedValueOnce(new Error('signature expired'))
    const workspace = createWorkspace(api)
    const file = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.file.value = file
    workspace.sourceZhCN.value = '官方授权'
    workspace.altZhCN.value = '封面'

    await workspace.upload()

    expect(workspace.stage.value).toBe('failed')
    expect(workspace.errorCode.value).toBe('upload-failed')
    expect(workspace.file.value).toBe(file)
    expect(workspace.sourceZhCN.value).toBe('官方授权')
    expect(workspace.canRetryComplete.value).toBe(false)
  })

  it('retries completion without creating or uploading another blob', async () => {
    const api = createApi()
    api.completeAssetUpload
      .mockRejectedValueOnce(new Error('metadata unavailable'))
      .mockResolvedValueOnce(readyAsset())
    const workspace = createWorkspace(api)
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '官方授权'
    workspace.altZhCN.value = '封面'

    await workspace.upload()
    expect(workspace.errorCode.value).toBe('complete-failed')
    expect(workspace.canRetryComplete.value).toBe(true)

    await workspace.retryComplete()

    expect(api.createAssetUpload).toHaveBeenCalledTimes(1)
    expect(api.uploadAssetBlob).toHaveBeenCalledTimes(1)
    expect(api.completeAssetUpload).toHaveBeenCalledTimes(2)
    expect(workspace.stage.value).toBe('succeeded')
  })

  it('appends pages without duplicate IDs and resets results for a status filter', async () => {
    const api = createApi()
    api.listAssets
      .mockResolvedValueOnce({ items: [readyAsset('asset_a'), readyAsset('asset_b')], nextCursor: 'next' })
      .mockResolvedValueOnce({ items: [readyAsset('asset_b'), readyAsset('asset_c')] })
      .mockResolvedValueOnce({ items: [readyAsset('asset_ready')] })
    const workspace = createWorkspace(api)

    await workspace.loadAssets()
    await workspace.loadMore()
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_a', 'asset_b', 'asset_c'])

    await workspace.setStatusFilter('ready')
    expect(api.listAssets).toHaveBeenLastCalledWith({ status: 'ready', limit: 50 })
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_ready'])
    expect(workspace.nextCursor.value).toBe('')
  })

  it('filters loaded assets by type or text without another request', async () => {
    const api = createApi()
    api.listAssets.mockResolvedValueOnce({
      items: [
        readyAsset('asset_cover', 'cover.webp', 'image/webp'),
        readyAsset('asset_song', 'song.mp3', 'audio/mpeg'),
      ],
    })
    const workspace = createWorkspace(api)
    await workspace.loadAssets()

    workspace.kindFilter.value = 'audio'
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_song'])
    workspace.kindFilter.value = 'all'
    workspace.searchText.value = 'COVER'
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_cover'])
    expect(api.listAssets).toHaveBeenCalledTimes(1)
  })

  it('does not update Vue state after its scope is disposed', async () => {
    const pending = deferred<{ items: AdminAsset[] }>()
    const api = createApi()
    api.listAssets.mockReturnValueOnce(pending.promise)
    const scope = effectScope()
    const workspace = scope.run(() => createWorkspace(api))!

    const loading = workspace.loadAssets()
    scope.stop()
    pending.resolve({ items: [readyAsset()] })
    await loading

    expect(workspace.assets.value).toEqual([])
    expect(workspace.loading.value).toBe(true)
  })

  it('does not let an older list response remove a newly completed upload', async () => {
    const pendingList = deferred<{ items: AdminAsset[] }>()
    const api = createApi()
    api.listAssets.mockReturnValueOnce(pendingList.promise)
    const workspace = createWorkspace(api)
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '官方授权'
    workspace.altZhCN.value = '封面'

    const loading = workspace.loadAssets()
    await workspace.upload()
    pendingList.resolve({ items: [] })
    await loading

    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_uploaded'])
    expect(workspace.loading.value).toBe(false)
  })

  it.each(['pending', 'deleted'] as const)('preserves an in-flight %s list when an upload completes', async (status) => {
    const pendingList = deferred<{ items: AdminAsset[], nextCursor: string }>()
    const api = createApi()
    api.listAssets.mockReturnValueOnce(pendingList.promise)
    const workspace = createWorkspace(api)
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '官方授权'
    workspace.altZhCN.value = '封面'

    const loading = workspace.setStatusFilter(status)
    await workspace.upload()

    expect(workspace.stage.value).toBe('succeeded')
    expect(workspace.assets.value).toEqual([])
    expect(workspace.loading.value).toBe(true)
    pendingList.resolve({ items: [
      { ...readyAsset('asset_other'), status },
      ...(status === 'pending' ? [{ ...readyAsset(), status }] : []),
    ], nextCursor: 'next-page' })
    await loading

    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_other'])
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_other'])
    expect(workspace.nextCursor.value).toBe('next-page')
    expect(workspace.loading.value).toBe(false)
  })
})

/** 使用默认成功响应构造可逐项覆盖的素材 API mock。 */
function createApi() {
  const upload: AdminAssetUpload = {
    asset: { ...readyAsset(), status: 'pending' },
    uploadUrl: 'https://upload.example.test/signed',
    headers: { 'Content-Type': 'image/webp' },
    expiresAt: '2026-10-09T09:00:00Z',
  }
  return {
    listAssets: vi.fn<AssetWorkspaceApi['listAssets']>(async () => ({ items: [] })),
    createAssetUpload: vi.fn<AssetWorkspaceApi['createAssetUpload']>(async () => upload),
    uploadAssetBlob: vi.fn<AssetWorkspaceApi['uploadAssetBlob']>(async () => undefined),
    completeAssetUpload: vi.fn<AssetWorkspaceApi['completeAssetUpload']>(async () => readyAsset()),
  }
}

/** 用固定依赖创建可重复测试的工作台。 */
function createWorkspace(api: ReturnType<typeof createApi>) {
  return useAssetWorkspace({
    apiBaseUrl: ref('https://api.yujian.me'),
    token: ref('session-token'),
    apiFactory: () => api,
    digest: async () => 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  })
}

/** 构造可被快照工作台消费的已确认素材。 */
function readyAsset(id = 'asset_uploaded', fileName = 'cover.webp', contentType = 'image/webp'): AdminAsset {
  return {
    id,
    src: `https://media.yujian.me/assets/${id}/source`,
    status: 'ready',
    metadata: {
      fileName,
      contentType,
      declaredSize: 5,
      checksum: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
    },
    rights: { source: { 'zh-CN': '官方授权' } },
    createdAt: '2026-10-09T08:00:00Z',
  }
}

/** 创建由测试显式完成的 Promise。 */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => {
    resolve = complete
  })
  return { promise, resolve }
}
