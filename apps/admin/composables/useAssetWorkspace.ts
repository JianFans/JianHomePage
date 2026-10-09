import type { Asset } from '@yujian/schema'
import { computed, getCurrentScope, onScopeDispose, ref, shallowRef, watch, type Ref } from 'vue'
import {
  createAdminApi,
  normalizeBaseUrl,
  type AdminApiOptions,
  type AdminAsset,
  type AdminAssetPage,
  type AdminAssetUpload,
  type AdminAssetUploadRequest,
  type AssetListOptions,
} from '../utils/admin-api'
import {
  AssetFileError,
  sha256File,
  validateAssetFile,
  type AssetFileErrorCode,
} from '../utils/asset-workbench'

export type AssetUploadStage = 'idle' | 'hashing' | 'creating' | 'uploading' | 'completing' | 'succeeded' | 'failed'
export type AssetWorkspaceErrorCode = AssetFileErrorCode | 'file-required' | 'form-invalid' | 'hashing-failed' | 'create-failed' | 'upload-failed' | 'complete-failed'

export interface AssetWorkspaceApi {
  listAssets(options?: AssetListOptions): Promise<AdminAssetPage>
  createAssetUpload(input: AdminAssetUploadRequest): Promise<AdminAssetUpload>
  uploadAssetBlob(upload: AdminAssetUpload, file: Blob): Promise<void>
  completeAssetUpload(assetId: string): Promise<AdminAsset>
}

export interface AssetWorkspaceOptions {
  apiBaseUrl: Readonly<Ref<string>>
  token: Readonly<Ref<string>>
  apiFactory?: (options: AdminApiOptions) => AssetWorkspaceApi
  digest?: (file: Blob) => Promise<string>
}

/** 管理素材直传、完成重试、分页筛选和作用域销毁后的异步写入保护。 */
export function useAssetWorkspace(options: AssetWorkspaceOptions) {
  const file = shallowRef<File | null>(null)
  const sourceZhCN = ref('')
  const sourceEn = ref('')
  const credit = ref('')
  const license = ref('')
  const altZhCN = ref('')
  const altEn = ref('')
  const stage = ref<AssetUploadStage>('idle')
  const errorCode = ref<AssetWorkspaceErrorCode | null>(null)
  const assets = ref<AdminAsset[]>([])
  const nextCursor = ref('')
  const statusFilter = ref<AdminAsset['status'] | ''>('')
  const kindFilter = ref<Asset['kind'] | 'all'>('all')
  const searchText = ref('')
  const loading = ref(false)
  const listError = ref(false)
  const pendingUpload = ref<AdminAssetUpload | null>(null)
  const blobUploaded = ref(false)
  const completedAsset = shallowRef<AdminAsset | null>(null)
  const apiFactory = options.apiFactory || createAdminApi
  const digest = options.digest || sha256File
  let uploadSequence = 0
  let listSequence = 0
  let disposed = false
  let uploadsDuringList = new Map<string, AdminAsset>()
  const connectionBaseUrl = computed(() => normalizeBaseUrl(options.apiBaseUrl.value))

  // Changing servers invalidates IDs, cursors and all in-flight results. Token
  // refreshes on the same server keep a pending upload available for retry.
  watch(connectionBaseUrl, () => {
    uploadSequence++
    listSequence++
    uploadsDuringList = new Map()
    assets.value = []
    nextCursor.value = ''
    pendingUpload.value = null
    completedAsset.value = null
    blobUploaded.value = false
    stage.value = 'idle'
    errorCode.value = null
    loading.value = false
    listError.value = false
  }, { flush: 'sync' })

  const canRetryComplete = computed(() => Boolean(pendingUpload.value && blobUploaded.value && errorCode.value === 'complete-failed'))
  const filteredAssets = computed(() => {
    const needle = searchText.value.trim().toLowerCase()
    return assets.value.filter((asset) => {
      if (!matchesStatus(asset)) return false
      if (kindFilter.value !== 'all' && kindFromContentType(asset.metadata.contentType) !== kindFilter.value) return false
      if (!needle) return true
      return [asset.id, asset.metadata.fileName, asset.metadata.contentType]
        .some(value => typeof value === 'string' && value.toLowerCase().includes(needle))
    })
  })

  if (getCurrentScope()) {
    onScopeDispose(() => {
      disposed = true
      uploadSequence++
      listSequence++
    })
  }

  /** 使用当前连接配置创建仅供本次操作使用的 API 客户端。 */
  function api(): AssetWorkspaceApi {
    return apiFactory({ baseUrl: options.apiBaseUrl.value, token: options.token.value })
  }

  /** 执行完整的文件预检、摘要、签名直传和完成确认流程。 */
  async function upload(): Promise<void> {
    const selected = file.value
    if (!selected) {
      fail('file-required')
      return
    }
    if (!sourceZhCN.value.trim() || !altZhCN.value.trim()) {
      fail('form-invalid')
      return
    }
    try {
      validateAssetFile(selected)
    } catch (error) {
      fail(error instanceof AssetFileError ? error.code : 'form-invalid')
      return
    }

    const sequence = ++uploadSequence
    const client = api()
    errorCode.value = null
    pendingUpload.value = null
    completedAsset.value = null
    blobUploaded.value = false
    try {
      stage.value = 'hashing'
      const checksum = await digest(selected)
      if (!activeUpload(sequence)) return

      stage.value = 'creating'
      const created = await client.createAssetUpload({
        fileName: selected.name,
        contentType: selected.type,
        size: selected.size,
        checksum,
        rights: uploadRights(),
      })
      if (!activeUpload(sequence)) return
      pendingUpload.value = created

      stage.value = 'uploading'
      await client.uploadAssetBlob(created, selected)
      if (!activeUpload(sequence)) return
      blobUploaded.value = true

      stage.value = 'completing'
      const completed = await client.completeAssetUpload(created.asset.id)
      if (!activeUpload(sequence)) return
      finishUpload(completed)
    } catch {
      if (!activeUpload(sequence)) return
      const code = stage.value === 'hashing'
        ? 'hashing-failed'
        : stage.value === 'creating'
          ? 'create-failed'
          : stage.value === 'uploading'
            ? 'upload-failed'
            : 'complete-failed'
      fail(code)
    }
  }

  /** 只重试已成功写入对象存储的完成确认步骤。 */
  async function retryComplete(): Promise<void> {
    const pending = pendingUpload.value
    if (!pending || !blobUploaded.value) return
    const sequence = ++uploadSequence
    errorCode.value = null
    stage.value = 'completing'
    try {
      const completed = await api().completeAssetUpload(pending.asset.id)
      if (!activeUpload(sequence)) return
      finishUpload(completed)
    } catch {
      if (activeUpload(sequence)) fail('complete-failed')
    }
  }

  /** 读取第一页或追加下一页，并按素材 ID 去重。 */
  async function loadAssets(append = false): Promise<void> {
    if (append && (!nextCursor.value || loading.value)) return
    const sequence = ++listSequence
    const completedDuringRequest = new Map<string, AdminAsset>()
    uploadsDuringList = completedDuringRequest
    loading.value = true
    listError.value = false
    try {
      const page = await api().listAssets({
        ...(statusFilter.value ? { status: statusFilter.value } : {}),
        limit: 50,
        ...(append && nextCursor.value ? { cursor: nextCursor.value } : {}),
      })
      if (!activeList(sequence)) return
      // An older page may contain the pending form of an upload completed while
      // it was in flight. Keep its cursor and other records, but use ready data.
      const incoming = [
        ...completedDuringRequest.values(),
        ...page.items.filter(asset => !completedDuringRequest.has(asset.id)),
      ].filter(matchesStatus)
      assets.value = append ? mergeAssets(assets.value, incoming) : incoming
      nextCursor.value = page.nextCursor || ''
    } catch {
      if (activeList(sequence)) listError.value = true
    } finally {
      if (activeList(sequence)) loading.value = false
    }
  }

  /** 在存在服务端游标时追加下一页。 */
  function loadMore(): Promise<void> {
    return loadAssets(true)
  }

  /** 切换服务端状态筛选，清空旧分页后重新读取第一页。 */
  function setStatusFilter(status: AdminAsset['status'] | ''): Promise<void> {
    statusFilter.value = status
    assets.value = []
    nextCursor.value = ''
    return loadAssets()
  }

  /** 判断指定上传序号仍属于当前有效作用域。 */
  function activeUpload(sequence: number): boolean {
    return !disposed && sequence === uploadSequence
  }

  /** 判断指定列表序号仍属于当前有效作用域。 */
  function activeList(sequence: number): boolean {
    return !disposed && sequence === listSequence
  }

  /** 把上传失败转换为稳定错误代码和终态。 */
  function fail(code: AssetWorkspaceErrorCode): void {
    errorCode.value = code
    stage.value = 'failed'
  }

  /** 按当前状态筛选合并已确认素材，保留正在读取的列表和分页。 */
  function finishUpload(asset: AdminAsset): void {
    completedAsset.value = asset
    if (loading.value) uploadsDuringList.set(asset.id, asset)
    assets.value = [asset, ...assets.value.filter(item => item.id !== asset.id)].filter(matchesStatus)
    pendingUpload.value = null
    blobUploaded.value = false
    errorCode.value = null
    stage.value = 'succeeded'
    file.value = null
    sourceZhCN.value = ''
    sourceEn.value = ''
    credit.value = ''
    license.value = ''
    altZhCN.value = ''
    altEn.value = ''
  }

  /** 默认隐藏已删除素材，显式筛选时仅保留对应状态。 */
  function matchesStatus(asset: AdminAsset): boolean {
    return statusFilter.value ? asset.status === statusFilter.value : asset.status !== 'deleted'
  }

  /** 从表单构造 canonical 权利结构并忽略空可选字段。 */
  function uploadRights(): Asset['rights'] {
    return {
      source: {
        'zh-CN': sourceZhCN.value.trim(),
        ...(sourceEn.value.trim() ? { en: sourceEn.value.trim() } : {}),
      },
      ...(credit.value.trim() ? { credit: credit.value.trim() } : {}),
      ...(license.value.trim() ? { license: license.value.trim() } : {}),
    }
  }

  return {
    connectionBaseUrl,
    file,
    sourceZhCN,
    sourceEn,
    credit,
    license,
    altZhCN,
    altEn,
    stage,
    errorCode,
    assets,
    completedAsset,
    nextCursor,
    statusFilter,
    kindFilter,
    searchText,
    loading,
    listError,
    canRetryComplete,
    filteredAssets,
    upload,
    retryComplete,
    loadAssets,
    loadMore,
    setStatusFilter,
  }
}

/** 依据服务端 canonical MIME 推导本地类型筛选值。 */
function kindFromContentType(contentType: unknown): Asset['kind'] | '' {
  if (contentType === 'image/webp') return 'image'
  if (contentType === 'image/gif') return 'gif'
  if (contentType === 'video/mp4') return 'video'
  if (contentType === 'audio/wav' || contentType === 'audio/mpeg') return 'audio'
  return ''
}

/** 以原顺序合并分页结果，并让后页同 ID 记录覆盖旧值。 */
function mergeAssets(current: readonly AdminAsset[], incoming: readonly AdminAsset[]): AdminAsset[] {
  const merged = new Map(current.map(asset => [asset.id, asset]))
  incoming.forEach(asset => merged.set(asset.id, asset))
  return [...merged.values()]
}
