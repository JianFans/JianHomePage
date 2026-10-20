import { effectScope, ref, watch } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { useAssetWorkspace, type AssetWorkspaceApi } from '../../composables/useAssetWorkspace'
import type { AdminApiOptions, AdminAsset, AdminAssetUpload } from '../../utils/admin-api'

describe('asset workspace state machine', () => {
  it('uses current session CSRF and forwards unauthorized responses without requiring a token', async () => {
    const api = createApi()
    const csrfToken = ref('csrf-first')
    const unauthorized = vi.fn()
    const factory = vi.fn<(options: AdminApiOptions) => AssetWorkspaceApi>(() => api)
    const workspace = useAssetWorkspace({ apiBaseUrl: ref(''), token: ref(''), csrfToken, onUnauthorized: unauthorized, apiFactory: factory })
    await workspace.loadAssets()
    expect(factory).toHaveBeenLastCalledWith(expect.objectContaining({ csrfToken: 'csrf-first', onUnauthorized: expect.any(Function) }))
    const oldUnauthorized = factory.mock.calls[0]![0].onUnauthorized!
    csrfToken.value = 'csrf-next'
    await workspace.loadAssets()
    expect(factory).toHaveBeenLastCalledWith(expect.objectContaining({ csrfToken: 'csrf-next' }))
    oldUnauthorized()
    expect(unauthorized).not.toHaveBeenCalled()
    factory.mock.calls[1]![0].onUnauthorized!()
    expect(unauthorized).toHaveBeenCalledOnce()
  })
  it('invalidates completion retries and library state when the API connection changes', /** 切换服务端后旧确认目标和分页失效，但未提交的表单仍可用于新上传。 */ async () => {
    const firstApi = createApi()
    firstApi.listAssets.mockResolvedValueOnce({ items: [readyAsset('asset_0f5179cb-849b-4786-84ae-ae17f29ffbda')], nextCursor: 'cursor-a' })
    firstApi.completeAssetUpload.mockRejectedValueOnce(new Error('temporary failure'))
    const secondApi = createApi()
    const baseUrl = ref('https://api-a.example')
    const workspace = useAssetWorkspace({
      apiBaseUrl: baseUrl, token: ref('token'), digest: async () => 'sha256:' + 'a'.repeat(64),
      apiFactory: options => options.baseUrl.includes('api-a') ? firstApi : secondApi,
    })
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '授权'
    workspace.altZhCN.value = '封面'
    await workspace.loadAssets()
    await workspace.upload()
    expect(workspace.canRetryComplete.value).toBe(true)

    baseUrl.value = 'https://api-b.example'
    expect(workspace.assets.value).toEqual([])
    expect(workspace.nextCursor.value).toBe('')
    expect(workspace.completedAsset.value).toBeNull()
    expect(workspace.stage.value).toBe('idle')
    expect(workspace.errorCode.value).toBeNull()
    expect(workspace.canRetryComplete.value).toBe(false)
    expect(workspace.file.value?.name).toBe('cover.webp')
    expect(workspace.altZhCN.value).toBe('封面')
    await workspace.retryComplete()
    await workspace.loadMore()
    expect(secondApi.completeAssetUpload).not.toHaveBeenCalled()
    expect(secondApi.listAssets).not.toHaveBeenCalled()
  })

  it('ignores an old connection list and does not continue an old upload after hashing', /** 用挂起的列表和摘要模拟连接竞争，确保迟到响应不能写回或创建旧连接上传。 */ async () => {
    const oldList = deferred<{ items: AdminAsset[], nextCursor: string }>()
    const hash = deferred<string>()
    const firstApi = createApi()
    firstApi.listAssets.mockReturnValueOnce(oldList.promise)
    const secondApi = createApi()
    secondApi.listAssets.mockResolvedValueOnce({ items: [readyAsset('asset_47f68d47-98a9-4288-be7c-c76c3dbcad67')] })
    const baseUrl = ref('https://api-a.example')
    const workspace = useAssetWorkspace({
      apiBaseUrl: baseUrl, token: ref('token'), digest: () => hash.promise,
      apiFactory: options => options.baseUrl.includes('api-a') ? firstApi : secondApi,
    })
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '授权'
    workspace.altZhCN.value = '封面'
    const loading = workspace.loadAssets()
    const uploading = workspace.upload()
    baseUrl.value = 'https://api-b.example'
    expect(workspace.loading.value).toBe(false)
    await workspace.loadAssets()
    oldList.resolve({ items: [readyAsset('asset_0f5179cb-849b-4786-84ae-ae17f29ffbda')], nextCursor: 'cursor-a' })
    hash.resolve('sha256:' + 'a'.repeat(64))
    await Promise.all([loading, uploading])
    expect(workspace.assets.value.map(item => item.id)).toEqual(['asset_47f68d47-98a9-4288-be7c-c76c3dbcad67'])
    expect(workspace.nextCursor.value).toBe('')
    expect(workspace.stage.value).toBe('idle')
    expect(firstApi.createAssetUpload).not.toHaveBeenCalled()
  })

  it('keeps the pending upload when only the token or trailing URL slash changes', /** 同一服务端的凭据刷新不丢失确认目标，重试使用更新后的客户端。 */ async () => {
    const firstApi = createApi()
    firstApi.completeAssetUpload.mockRejectedValueOnce(new Error('temporary failure'))
    const refreshedApi = createApi()
    const baseUrl = ref('https://api-a.example')
    const token = ref('expired-token')
    const workspace = useAssetWorkspace({
      apiBaseUrl: baseUrl, token, digest: async () => 'sha256:' + 'a'.repeat(64),
      apiFactory: options => options.token === 'expired-token' ? firstApi : refreshedApi,
    })
    workspace.file.value = new File(['asset'], 'cover.webp', { type: 'image/webp' })
    workspace.sourceZhCN.value = '授权'
    workspace.altZhCN.value = '封面'
    await workspace.upload()
    baseUrl.value += '/'
    token.value = 'refreshed-token'
    expect(workspace.canRetryComplete.value).toBe(true)
    await workspace.retryComplete()
    expect(refreshedApi.completeAssetUpload).toHaveBeenCalledWith('asset_a2a76b6c-d817-4bb7-8064-a9efbceb644c')
    expect(workspace.stage.value).toBe('succeeded')
  })

  it('runs the successful upload stages in order and adds the ready asset', /** 同步收集阶段转换，验证权利信息、一次直传和确认后的列表及文件清理。 */ async () => {
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
    expect(api.completeAssetUpload).toHaveBeenCalledWith('asset_a2a76b6c-d817-4bb7-8064-a9efbceb644c')
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_a2a76b6c-d817-4bb7-8064-a9efbceb644c'])
    expect(workspace.file.value).toBeNull()
  })

  it('retains the form after blob upload failure so a new signature can be requested', /** 直传失败保留用户输入，同时禁止将未上传的对象当作可重试确认的素材。 */ async () => {
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

  it('retries completion without creating or uploading another blob', /** 首次确认失败后仅重试元数据确认，避免重新分配素材或重复传输文件。 */ async () => {
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

  it('appends pages without duplicate IDs and resets results for a status filter', /** 重叠分页按 ID 去重，状态变化则重新请求固定页大小并清除旧游标。 */ async () => {
    const api = createApi()
    api.listAssets
      .mockResolvedValueOnce({ items: [readyAsset('asset_0f5179cb-849b-4786-84ae-ae17f29ffbda'), readyAsset('asset_47f68d47-98a9-4288-be7c-c76c3dbcad67')], nextCursor: 'next' })
      .mockResolvedValueOnce({ items: [readyAsset('asset_47f68d47-98a9-4288-be7c-c76c3dbcad67'), readyAsset('asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6')] })
      .mockResolvedValueOnce({ items: [readyAsset('asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17')] })
    const workspace = createWorkspace(api)

    await workspace.loadAssets()
    await workspace.loadMore()
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_0f5179cb-849b-4786-84ae-ae17f29ffbda', 'asset_47f68d47-98a9-4288-be7c-c76c3dbcad67', 'asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6'])

    await workspace.setStatusFilter('ready')
    expect(api.listAssets).toHaveBeenLastCalledWith({ status: 'ready', limit: 50 })
    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17'])
    expect(workspace.nextCursor.value).toBe('')
  })

  it('filters loaded assets by type or text without another request', /** 类型和大小写不敏感的文本搜索只处理已加载记录，不触发额外网络请求。 */ async () => {
    const api = createApi()
    api.listAssets.mockResolvedValueOnce({
      items: [
        readyAsset('asset_1640be1f-25cb-4df5-9f39-b7bffd72f5a1', 'cover.webp', 'image/webp'),
        readyAsset('asset_15cae191-c6eb-4a04-a805-2bd5b0242011', 'song.mp3', 'audio/mpeg'),
      ],
    })
    const workspace = createWorkspace(api)
    await workspace.loadAssets()

    workspace.kindFilter.value = 'audio'
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_15cae191-c6eb-4a04-a805-2bd5b0242011'])
    workspace.kindFilter.value = 'all'
    workspace.searchText.value = 'COVER'
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_1640be1f-25cb-4df5-9f39-b7bffd72f5a1'])
    expect(api.listAssets).toHaveBeenCalledTimes(1)
  })

  it('does not update Vue state after its scope is disposed', /** 销毁作用域后才完成列表请求，验证组件卸载后的响应不会再修改响应式状态。 */ async () => {
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

  it('does not let an older list response remove a newly completed upload', /** 上传确认先于旧列表返回时，迟到的空列表不能覆盖刚加入的 ready 素材。 */ async () => {
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

    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_a2a76b6c-d817-4bb7-8064-a9efbceb644c'])
    expect(workspace.loading.value).toBe(false)
  })

  it.each(['pending', 'deleted'] as const)('preserves an in-flight %s list when an upload completes', /** 确认期间保留其他状态的分页请求，排除已转 ready 的记录但保留其他素材和游标。 */ async (status) => {
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
      { ...readyAsset('asset_effdd871-571a-464b-b609-ba85347b5360'), status },
      ...(status === 'pending' ? [{ ...readyAsset(), status }] : []),
    ], nextCursor: 'next-page' })
    await loading

    expect(workspace.assets.value.map(asset => asset.id)).toEqual(['asset_effdd871-571a-464b-b609-ba85347b5360'])
    expect(workspace.filteredAssets.value.map(asset => asset.id)).toEqual(['asset_effdd871-571a-464b-b609-ba85347b5360'])
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
function readyAsset(id = 'asset_a2a76b6c-d817-4bb7-8064-a9efbceb644c', fileName = 'cover.webp', contentType = 'image/webp'): AdminAsset {
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
