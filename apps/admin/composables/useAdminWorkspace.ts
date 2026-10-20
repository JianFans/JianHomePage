import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'
import {
  createAdminApi,
  parseSnapshotJSON,
  snapshotJSONErrorMessage,
  type AdminPublishJob,
  type AdminVersion,
  type AdminVersionSummary,
} from '../utils/admin-api'
import { idleWorkflow, workflowError, workflowSuccess, type WorkflowState } from '../utils/admin-workflow'
import { createIdempotencyKey, createOperationKeyStore, type PublishOperation } from '../utils/idempotency'
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
import { createInitialSnapshot } from '../utils/initial-snapshot'
import { usePublishMonitor } from './usePublishMonitor'

const EDITOR_ANALYSIS_DEBOUNCE_MS = 250

/**
 * 管理内容编辑、导入导出、审核和发布状态，并按当前语言生成提示。
 * 界面分析经过防抖，产生副作用的保存与导出始终重新读取当前文本。
 */
export function useAdminWorkspace(locale: Readonly<Ref<AdminLocale>> = ref<AdminLocale>('zh-CN'), session: {
  csrfToken?: Readonly<Ref<string>>
  active?: Readonly<Ref<boolean>>
  busy?: Readonly<Ref<boolean>>
  onUnauthorized?: () => void
} = {}) {
  const runtime = useRuntimeConfig()
  const apiBaseUrl = ref(String(runtime.public.apiBaseUrl || ''))
  const token = ref('')
  const versionId = ref('')
  const version = ref<AdminVersion | null>(null)
  const versions = ref<AdminVersionSummary[]>([])
  const nextCursor = ref('')
  const listing = ref(false)
  const listError = ref(false)
  const draftName = ref('')
  const publishJob = ref<AdminPublishJob | null>(null)
  const publishOperation = ref<PublishOperation | null>(null)
  const editorText = ref('{}')
  const savedText = ref(editorText.value)
  const rejectReason = ref('')
  const workflow = ref<WorkflowState>(idleWorkflow())
  const importing = ref(false)
  const operationKeys = createOperationKeyStore()
  const retryKeys = new Map<string, string>()
  let importSequence = 0
  let listSequence = 0
  let epoch = 0
  let disposed = false
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
    disposed = true
    epoch++
    importSequence++
    listSequence++
    if (editorAnalysisTimer) clearTimeout(editorAnalysisTimer)
  })
  if (session.active) watch(session.active, (active) => {
    if (active) return
    epoch++
    importSequence++
    listSequence++
    importing.value = false
    listing.value = false
    workflow.value = idleWorkflow()
  }, { flush: 'sync' })

  const editorAnalysis = computed(() => analyzeSnapshotText(debouncedEditorText.value))
  /** Expose the existing debounce baseline without performing another snapshot analysis. */
  const analyzedText = computed(() => debouncedEditorText.value)
  const parsedEditor = computed(() => parseSnapshotJSON(debouncedEditorText.value, locale.value))
  const busy = computed(() => session.busy?.value === true || importing.value || ['loading', 'saving', 'reviewing', 'publishing'].includes(workflow.value.status))
  const dirty = computed(() => editorText.value !== savedText.value)
  const editable = computed(() => (!version.value || version.value.status === 'draft') && session.active?.value !== false)
  const canSave = computed(() => editable.value && Boolean(editorAnalysis.value.snapshot) && !busy.value)
  const canSubmitReview = computed(() => version.value?.status === 'draft' && !busy.value && !dirty.value)
  const canApprove = computed(() => version.value?.status === 'in_review' && !version.value.reviewApproved && !busy.value && !dirty.value)
  const canReject = computed(() => canApprove.value && Boolean(rejectReason.value.trim()))
  const canPublish = computed(() => version.value?.status === 'in_review' && version.value.reviewApproved === true && !busy.value && !dirty.value)
  const canRollback = computed(() => (version.value?.status === 'published' || version.value?.status === 'archived') && !busy.value)
  const publishMonitor = usePublishMonitor({
    job: publishJob, active: session.active, paused: busy,
    read: (id, signal) => api().getPublish(id, signal),
    latest: signal => api().latestPublish(signal),
    updated: async (job, current, signal) => {
      publishOperation.value = job.operation || publishOperation.value
      if (publishOperation.value) operationKeys.settle(publishOperation.value, job.versionId, job.status)
      if (job.status === 'succeeded' || job.status === 'failed') await synchronizePublishVersions(job, current, signal)
    },
  })

  /** 使用最新连接地址和令牌创建一次性 API 客户端。 */
  function api() {
    const currentEpoch = epoch
    const currentCsrf = session.csrfToken?.value
    return createAdminApi({ baseUrl: apiBaseUrl.value, token: token.value, csrfToken: currentCsrf,
      onUnauthorized: () => {
        if (!disposed && currentEpoch === epoch && currentCsrf === session.csrfToken?.value) session.onUnauthorized?.()
      } })
  }

  /** 统一维护异步操作的忙碌、成功和安全错误状态。 */
  async function run<T>(status: WorkflowState['status'], operation: () => Promise<T>, successMessage: string): Promise<T | null> {
    if (busy.value || disposed || session.active?.value === false) return null
    const currentEpoch = epoch
    workflow.value = { status, message: '', requestId: '' }
    try {
      const result = await operation()
      if (disposed || epoch !== currentEpoch) return null
      workflow.value = workflowSuccess(successMessage)
      return result
    } catch (error) {
      if (disposed || epoch !== currentEpoch) return null
      workflow.value = workflowError(error)
      return null
    }
  }

  /** 将服务端版本同步到版本标识与 JSON 编辑器。 */
  function setVersion(next: AdminVersion, replaceText = true) {
    // A list started before this read/write may contain an older revision.
    listSequence++
    listing.value = false
    version.value = next
    versionId.value = next.id
    draftName.value = next.name || next.id
    const { snapshot, ...summary } = next
    const index = versions.value.findIndex(item => item.id === next.id)
    if (index >= 0) versions.value[index] = summary
    else versions.value.unshift(summary)
    savedText.value = JSON.stringify(snapshot, null, 2)
    if (replaceText) editorText.value = savedText.value
  }

  /** 分页只读取摘要；重新刷新使之前的页请求失效，避免迟到数据回滚列表。 */
  async function listVersions(append = false) {
    if (disposed || session.active?.value === false || (append && (!nextCursor.value || listing.value))) return
    const sequence = ++listSequence
    const currentEpoch = epoch
    listing.value = true
    listError.value = false
    try {
      const page = await api().listVersions({ limit: 20, cursor: append ? nextCursor.value : undefined })
      if (disposed || sequence !== listSequence || currentEpoch !== epoch) return
      const unique = new Map<string, AdminVersionSummary>()
      for (const item of [...(append ? versions.value : []), ...page.items]) if (!unique.has(item.id)) unique.set(item.id, item)
      versions.value = [...unique.values()]
      nextCursor.value = page.nextCursor || ''
    } catch {
      if (disposed || sequence !== listSequence || currentEpoch !== epoch) return
      listError.value = true
    } finally {
      if (!disposed && sequence === listSequence && currentEpoch === epoch) listing.value = false
    }
  }

  /** 确认空库后准备未保存的首份草稿，不覆盖导入、恢复输入或已选版本。 */
  function initializeDraft(): boolean {
    if (disposed || busy.value || !editable.value || listError.value || listing.value
      || version.value || versions.value.length || nextCursor.value || dirty.value || editorText.value !== '{}') return false
    editorText.value = JSON.stringify(createInitialSnapshot(), null, 2)
    draftName.value = locale.value === 'en' ? 'Initial version' : '初始版本'
    return true
  }

  /** 服务端读取源快照并生成独立版本；客户端不参与复制内容或审核状态。 */
  async function cloneVersion(id: string, name: string) {
    if (!id.trim() || !name.trim()) return false
    const result = await run('saving', () => api().cloneVersion(id, name.trim()), locale.value === 'en' ? 'Draft created' : '草稿已创建')
    if (!result) return false
    setVersion(result)
    return true
  }

  /** 校验版本 ID 后载入服务端内容版本。 */
  async function loadVersion() {
    if (busy.value) return
    if (!versionId.value.trim()) {
      workflow.value = workflowError({ message: locale.value === 'en' ? 'Choose a version first' : '请先填写版本 ID' })
      return
    }
    const result = await run('loading', () => api().getVersion(versionId.value.trim()), locale.value === 'en' ? 'Version loaded' : '已载入版本')
    if (result) setVersion(result)
  }

  /**
   * 直接校验当前编辑器文本，并创建新草稿或乐观更新已有草稿。
   */
  async function saveDraft() {
    if (busy.value || !editable.value) return
    const submittedText = editorText.value
    const currentVersion = version.value
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
    if (currentVersion) {
      const result = await run('saving', () => api().updateVersion(currentVersion.id, currentVersion.revision, snapshot), locale.value === 'en' ? 'Draft saved' : '草稿已保存')
      if (result) setVersion(result, editorText.value === submittedText)
      return
    }
    const result = await run('saving', () => api().createVersion(snapshot, draftName.value.trim() || undefined), locale.value === 'en' ? 'Draft created' : '草稿已创建')
    if (result) setVersion(result, editorText.value === submittedText)
  }

  /**
   * 导入本地快照，并用单调序号保证只有最后一次异步导入可以更新状态。
   */
  async function importSnapshot(file: SnapshotImportFile, locale: AdminLocale = 'zh-CN') {
    if (!editable.value || disposed || ['loading', 'saving', 'reviewing', 'publishing'].includes(workflow.value.status)) return
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
    if (!version.value || dirty.value || busy.value) return
    const result = await run('reviewing', () => api().submitReview(version.value!.id, version.value!.revision), locale.value === 'en' ? 'Submitted for review' : '已提交审核')
    if (result) setVersion(result)
  }

  /** 批准当前审核版本，并同步审核状态。 */
  async function approveReview() {
    if (!canApprove.value) return
    const result = await run('reviewing', () => api().approveReview(version.value!.id, version.value!.revision), locale.value === 'en' ? 'Review approved' : '审核已通过')
    if (result) setVersion(result)
  }

  /** 校验退回原因后将当前审核版本退回草稿。 */
  async function rejectReview() {
    if (!canApprove.value) return
    if (!version.value || !rejectReason.value.trim()) {
      workflow.value = workflowError({ message: locale.value === 'en' ? 'Enter a return reason' : '请填写退回原因' })
      return
    }
    const result = await run('reviewing', () => api().rejectReview(version.value!.id, version.value!.revision, rejectReason.value.trim()), locale.value === 'en' ? 'Returned to draft' : '已退回草稿')
    if (result) setVersion(result)
  }

  /** 使用可复用幂等键为当前版本创建发布任务。 */
  async function publish() {
    if (!version.value || dirty.value || busy.value) return
    const current = version.value
    const key = operationKeys.get('publish', current.id)
    const accepted = await runPublishJob('publish', () => api().publish(current.id, key), locale.value === 'en' ? 'Publish job created' : '发布任务已创建')
    if (!accepted && workflow.value.status === 'error') await publishMonitor.restore()
  }

  /** 刷新当前发布任务，并在终态释放对应幂等键。 */
  async function refreshPublish() {
    if (!publishJob.value) return
    await runPublishJob(publishOperation.value, () => api().refreshPublish(publishJob.value!.id), locale.value === 'en' ? 'Publish status refreshed' : '发布状态已刷新')
  }

  /** 为当前已发布或归档版本创建幂等回滚任务。 */
  async function rollback() {
    if (!version.value) return
    const current = version.value
    const key = operationKeys.get('rollback', current.id)
    await runPublishJob('rollback', () => api().rollback(current.id, key), locale.value === 'en' ? 'Rollback job created' : '回滚任务已创建')
  }

  /** 失败重试使用新任务与新幂等键，保留原操作类型、审核约束和历史记录。 */
  async function retryPublish() {
    const job = publishJob.value
    if (!job || job.status !== 'failed' || busy.value || dirty.value) return
    const operation = job.operation || publishOperation.value || 'publish'
    const key = retryKeys.get(job.id) || createIdempotencyKey(operation)
    retryKeys.set(job.id, key)
    const accepted = await runPublishJob(operation, () => operation === 'rollback' ? api().rollback(job.versionId, key) : api().publish(job.versionId, key), locale.value === 'en' ? 'Retry created' : '已创建重试任务')
    if (accepted) retryKeys.delete(job.id)
    else if (workflow.value.status === 'error') await publishMonitor.restore()
  }

  /** 先保留已接受的核实结果，后续读取失败只重试状态同步，不重复恢复写入。 */
  async function recoverPublish(input: { note: string; deploymentId?: string; confirmedAbsent?: true }): Promise<boolean> {
    const job = publishJob.value
    if (!job?.buildId || job.status !== 'building' || job.build?.phase !== 'submitting' || busy.value || dirty.value) return false
    const currentEpoch = epoch
    const accepted = await run('publishing', async () => {
      publishMonitor.stop()
      const recovered = await api().recoverPublish(job.buildId!, input)
      if (disposed || epoch !== currentEpoch || publishJob.value?.id !== job.id || publishJob.value.buildId !== job.buildId) return false
      publishJob.value = { ...job, build: { ...job.build!, phase: recovered.phase, deploymentId: recovered.deploymentId, error: recovered.error } }
      return true
    }, locale.value === 'en' ? 'Recovery recorded' : '核实结果已记录')
    if (accepted) await publishMonitor.refresh()
    return accepted === true
  }

  /** 任务刷新只更新关联版本，不替换当前选择，也不覆盖未保存的 JSON 输入。 */
  async function synchronizePublishVersions(job: AdminPublishJob, current: () => boolean, signal?: AbortSignal) {
    const relatedIds = new Set([job.versionId])
    if (job.status === 'succeeded') {
      for (const item of versions.value) if (item.status === 'published') relatedIds.add(item.id)
      if (version.value?.status === 'published') relatedIds.add(version.value.id)
    }
    for (const id of relatedIds) {
      const next = await api().getVersion(id, signal)
      if (!current()) return
      if (version.value?.id === next.id) setVersion(next, !dirty.value)
      else {
        listSequence++
        listing.value = false
        const { snapshot, ...summary } = next
        void snapshot
        const index = versions.value.findIndex(item => item.id === next.id)
        if (index < 0) versions.value.unshift(summary)
        else versions.value[index] = summary
      }
    }
  }

  /** 先保留已接受任务，再同步关联版本；读取失败不能丢失任务或重发云操作。 */
  async function runPublishJob(operation: PublishOperation | null, request: () => Promise<AdminPublishJob>, message: string) {
    const currentEpoch = epoch
    let accepted = false
    await run('publishing', async () => {
      const job = await request()
      if (disposed || currentEpoch !== epoch) return
      accepted = true
      publishOperation.value = operation
      publishJob.value = job
      if (operation) operationKeys.settle(operation, job.versionId, job.status)
      await synchronizePublishVersions(job, () => !disposed && currentEpoch === epoch)
    }, message)
    return accepted
  }

  /** 仅在服务端退出成功后清理内容和基线，失效会话仍保留可导出的输入。 */
  function clear() {
    publishMonitor.stop()
    retryKeys.clear()
    epoch++
    importSequence++
    listSequence++
    version.value = null
    versionId.value = ''
    versions.value = []
    nextCursor.value = ''
    draftName.value = ''
    publishJob.value = null
    publishOperation.value = null
    editorText.value = savedText.value = '{}'
    rejectReason.value = ''
    listError.value = false
    workflow.value = idleWorkflow()
  }

  return {
    clear,
    apiBaseUrl,
    token,
    versionId,
    version,
    versions,
    nextCursor,
    listing,
    listError,
    draftName,
    editable,
    publishJob,
    publishRefreshing: publishMonitor.refreshing,
    publishReadError: publishMonitor.error,
    restorePublish: publishMonitor.restore,
    editorText,
    rejectReason,
    workflow,
    importing,
    editorAnalysis,
    analyzedText,
    parsedEditor,
    busy,
    dirty,
    canSave,
    canSubmitReview,
    canApprove,
    canReject,
    canPublish,
    canRollback,
    loadVersion,
    listVersions,
    initializeDraft,
    cloneVersion,
    saveDraft,
    importSnapshot,
    exportSnapshot,
    submitReview,
    approveReview,
    rejectReview,
    publish,
    refreshPublish,
    retryPublish,
    recoverPublish,
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
