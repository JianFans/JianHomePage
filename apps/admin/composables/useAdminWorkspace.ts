import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import {
  createAdminApi,
  parseSnapshotJSON,
  snapshotJSONErrorMessage,
  type AdminPublishJob,
  type AdminVersion,
} from '../utils/admin-api'
import { idleWorkflow, workflowError, workflowSuccess, type WorkflowState } from '../utils/admin-workflow'
import { createOperationKeyStore, type PublishOperation } from '../utils/idempotency'
import {
  analyzeSnapshotText,
  createSnapshotExport,
  readSnapshotImport,
  SnapshotImportError,
  type SnapshotExport,
  type SnapshotImportErrorCode,
  type SnapshotImportFile,
} from '../utils/snapshot-workbench'
import type { AdminLocale } from '../utils/admin-locale'

const EDITOR_ANALYSIS_DEBOUNCE_MS = 250

/**
 * 管理内容编辑、导入导出、审核和发布状态，并按当前语言生成提示。
 * 界面分析经过防抖，产生副作用的保存与导出始终重新读取当前文本。
 */
export function useAdminWorkspace(locale: Readonly<Ref<AdminLocale>> = ref<AdminLocale>('zh-CN')) {
  const runtime = useRuntimeConfig()
  const apiBaseUrl = ref(String(runtime.public.apiBaseUrl || ''))
  const token = ref('')
  const versionId = ref('')
  const version = ref<AdminVersion | null>(null)
  const publishJob = ref<AdminPublishJob | null>(null)
  const publishOperation = ref<PublishOperation | null>(null)
  const editorText = ref('{}')
  const rejectReason = ref('')
  const workflow = ref<WorkflowState>(idleWorkflow())
  const importing = ref(false)
  const operationKeys = createOperationKeyStore()
  let importSequence = 0
  let editorAnalysisTimer: ReturnType<typeof setTimeout> | undefined

  const debouncedEditorText = ref(editorText.value)
  watch(editorText, (value) => {
    if (editorAnalysisTimer) clearTimeout(editorAnalysisTimer)
    editorAnalysisTimer = setTimeout(() => {
      debouncedEditorText.value = value
      editorAnalysisTimer = undefined
    }, EDITOR_ANALYSIS_DEBOUNCE_MS)
  }, { flush: 'sync' })
  onScopeDispose(() => {
    if (editorAnalysisTimer) clearTimeout(editorAnalysisTimer)
  })

  const editorAnalysis = computed(() => analyzeSnapshotText(debouncedEditorText.value))
  const parsedEditor = computed(() => parseSnapshotJSON(debouncedEditorText.value, locale.value))
  const busy = computed(() => importing.value || ['loading', 'saving', 'reviewing', 'publishing'].includes(workflow.value.status))
  const canSave = computed(() => Boolean(editorAnalysis.value.snapshot) && !busy.value)
  const canSubmitReview = computed(() => version.value?.status === 'draft' && !busy.value)
  const canApprove = computed(() => version.value?.status === 'in_review' && !version.value.reviewApproved && !busy.value)
  const canPublish = computed(() => version.value?.status === 'in_review' && version.value.reviewApproved === true && !busy.value)
  const canRollback = computed(() => (version.value?.status === 'published' || version.value?.status === 'archived') && !busy.value)

  /** 使用最新连接地址和令牌创建一次性 API 客户端。 */
  function api() {
    return createAdminApi({ baseUrl: apiBaseUrl.value, token: token.value })
  }

  /** 统一维护异步操作的忙碌、成功和安全错误状态。 */
  async function run<T>(status: WorkflowState['status'], operation: () => Promise<T>, successMessage: string): Promise<T | null> {
    workflow.value = { status, message: '', requestId: '' }
    try {
      const result = await operation()
      workflow.value = workflowSuccess(successMessage)
      return result
    } catch (error) {
      workflow.value = workflowError(error)
      return null
    }
  }

  /** 将服务端版本同步到版本标识与 JSON 编辑器。 */
  function setVersion(next: AdminVersion) {
    version.value = next
    versionId.value = next.id
    editorText.value = JSON.stringify(next.snapshot, null, 2)
  }

  /** 校验版本 ID 后载入服务端内容版本。 */
  async function loadVersion() {
    if (!versionId.value.trim()) {
      workflow.value = workflowError({ message: '请先填写版本 ID' })
      return
    }
    const result = await run('loading', () => api().getVersion(versionId.value.trim()), '已载入版本')
    if (result) setVersion(result)
  }

  /**
   * 直接校验当前编辑器文本，并创建新草稿或乐观更新已有草稿。
   */
  async function saveDraft() {
    const currentAnalysis = analyzeSnapshotText(editorText.value)
    const snapshot = currentAnalysis.snapshot as unknown as Record<string, unknown> | null
    if (!snapshot) {
      workflow.value = workflowError({
        message: parseSnapshotJSON(editorText.value, locale.value).error
          || issueMessage(currentAnalysis.issues[0], locale.value)
          || invalidSnapshotMessage(locale.value),
      })
      return
    }
    if (version.value) {
      const result = await run('saving', () => api().updateVersion(version.value!.id, version.value!.revision, snapshot), '草稿已保存')
      if (result) setVersion(result)
      return
    }
    const result = await run('saving', () => api().createVersion(snapshot), '草稿已创建')
    if (result) setVersion(result)
  }

  /**
   * 导入本地快照，并用单调序号保证只有最后一次异步导入可以更新状态。
   */
  async function importSnapshot(file: SnapshotImportFile, locale: AdminLocale = 'zh-CN') {
    const sequence = ++importSequence
    importing.value = true
    try {
      const contents = await readSnapshotImport(file)
      if (sequence !== importSequence) return
      editorText.value = contents
      workflow.value = workflowSuccess(locale === 'en' ? 'Snapshot imported' : '已导入快照')
    } catch (error) {
      if (sequence !== importSequence) return
      workflow.value = error instanceof SnapshotImportError
        ? workflowError({ message: snapshotImportErrorMessage(error.code, locale) })
        : workflowError(error)
    } finally {
      if (sequence === importSequence) importing.value = false
    }
  }

  /** 从当前编辑器文本导出快照，文本无效时拒绝生成文件。 */
  function exportSnapshot(): SnapshotExport | null {
    const snapshot = analyzeSnapshotText(editorText.value).snapshot
    return snapshot ? createSnapshotExport(snapshot) : null
  }

  /** 将当前草稿提交审核，并同步返回的新修订。 */
  async function submitReview() {
    if (!version.value) return
    const result = await run('reviewing', () => api().submitReview(version.value!.id, version.value!.revision), '已提交审核')
    if (result) setVersion(result)
  }

  /** 批准当前审核版本，并同步审核状态。 */
  async function approveReview() {
    if (!version.value) return
    const result = await run('reviewing', () => api().approveReview(version.value!.id, version.value!.revision), '审核已通过')
    if (result) setVersion(result)
  }

  /** 校验退回原因后将当前审核版本退回草稿。 */
  async function rejectReview() {
    if (!version.value || !rejectReason.value.trim()) {
      workflow.value = workflowError({ message: '请填写退回原因' })
      return
    }
    const result = await run('reviewing', () => api().rejectReview(version.value!.id, version.value!.revision, rejectReason.value.trim()), '已退回草稿')
    if (result) setVersion(result)
  }

  /** 使用可复用幂等键为当前版本创建发布任务。 */
  async function publish() {
    if (!version.value) return
    const current = version.value
    const key = operationKeys.get('publish', current.id)
    const result = await run('publishing', () => api().publish(current.id, key), '发布任务已创建')
    if (result) setPublishJob('publish', result)
  }

  /** 刷新当前发布任务，并在终态释放对应幂等键。 */
  async function refreshPublish() {
    if (!publishJob.value) return
    const result = await run('publishing', () => api().refreshPublish(publishJob.value!.id), '发布状态已刷新')
    if (result) {
      publishJob.value = result
      if (publishOperation.value) {
        operationKeys.settle(publishOperation.value, result.versionId, result.status)
      }
    }
  }

  /** 为当前已发布或归档版本创建幂等回滚任务。 */
  async function rollback() {
    if (!version.value) return
    const current = version.value
    const key = operationKeys.get('rollback', current.id)
    const result = await run('publishing', () => api().rollback(current.id, key), '回滚任务已创建')
    if (result) setPublishJob('rollback', result)
  }

  /** 记录发布任务，并依据任务状态维护操作幂等键。 */
  function setPublishJob(operation: PublishOperation, job: AdminPublishJob) {
    publishOperation.value = operation
    publishJob.value = job
    operationKeys.settle(operation, job.versionId, job.status)
  }

  return {
    apiBaseUrl,
    token,
    versionId,
    version,
    publishJob,
    editorText,
    rejectReason,
    workflow,
    importing,
    editorAnalysis,
    parsedEditor,
    busy,
    canSave,
    canSubmitReview,
    canApprove,
    canPublish,
    canRollback,
    loadVersion,
    saveDraft,
    importSnapshot,
    exportSnapshot,
    submitReview,
    approveReview,
    rejectReview,
    publish,
    refreshPublish,
    rollback,
  }
}

/** 将首个编辑器或内容契约问题转换为本地化提示。 */
function issueMessage(issue: { path: string; code: string } | undefined, locale: AdminLocale): string {
  if (!issue) return ''
  if (issue.code === 'invalid-json' || issue.code === 'object-root') {
    return snapshotJSONErrorMessage(issue.code, locale)
  }
  return locale === 'en' ? `Invalid snapshot: ${issue.path}` : `快照无效：${issue.path}`
}

/** 返回无具体诊断时使用的本地化快照错误消息。 */
function invalidSnapshotMessage(locale: AdminLocale): string {
  return locale === 'en' ? 'Snapshot is invalid' : '快照无效'
}

/** 将稳定的导入错误代码映射为当前界面语言。 */
function snapshotImportErrorMessage(code: SnapshotImportErrorCode, locale: AdminLocale): string {
  const messages = locale === 'en'
    ? {
        'invalid-extension': 'Choose a JSON file',
        'empty-file': 'JSON file cannot be empty',
        'file-too-large': 'JSON file cannot exceed 2 MiB',
        'read-failed': 'Unable to read JSON file',
      }
    : {
        'invalid-extension': '请选择 JSON 文件',
        'empty-file': 'JSON 文件不能为空',
        'file-too-large': 'JSON 文件不能超过 2 MiB',
        'read-failed': '无法读取 JSON 文件',
      }
  return messages[code]
}
