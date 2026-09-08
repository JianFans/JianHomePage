import { computed, ref } from 'vue'
import {
  createAdminApi,
  parseSnapshotJSON,
  type AdminPublishJob,
  type AdminVersion,
} from '../utils/admin-api'
import { idleWorkflow, workflowError, workflowSuccess, type WorkflowState } from '../utils/admin-workflow'
import { createOperationKeyStore, type PublishOperation } from '../utils/idempotency'
import {
  analyzeSnapshotText,
  createSnapshotExport,
  readSnapshotImport,
  type SnapshotExport,
  type SnapshotImportFile,
} from '../utils/snapshot-workbench'

export function useAdminWorkspace() {
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
  const operationKeys = createOperationKeyStore()

  const editorAnalysis = computed(() => analyzeSnapshotText(editorText.value))
  const parsedEditor = computed(() => parseSnapshotJSON(editorText.value))
  const busy = computed(() => ['loading', 'saving', 'reviewing', 'publishing'].includes(workflow.value.status))
  const canSave = computed(() => Boolean(editorAnalysis.value.snapshot) && !busy.value)
  const canSubmitReview = computed(() => version.value?.status === 'draft' && !busy.value)
  const canApprove = computed(() => version.value?.status === 'in_review' && !version.value.reviewApproved && !busy.value)
  const canPublish = computed(() => version.value?.status === 'in_review' && version.value.reviewApproved === true && !busy.value)
  const canRollback = computed(() => (version.value?.status === 'published' || version.value?.status === 'archived') && !busy.value)

  function api() {
    return createAdminApi({ baseUrl: apiBaseUrl.value, token: token.value })
  }

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

  function setVersion(next: AdminVersion) {
    version.value = next
    versionId.value = next.id
    editorText.value = JSON.stringify(next.snapshot, null, 2)
  }

  async function loadVersion() {
    if (!versionId.value.trim()) {
      workflow.value = workflowError({ message: '请先填写版本 ID' })
      return
    }
    const result = await run('loading', () => api().getVersion(versionId.value.trim()), '已载入版本')
    if (result) setVersion(result)
  }

  async function saveDraft() {
    const snapshot = editorAnalysis.value.snapshot as unknown as Record<string, unknown> | null
    if (!snapshot) {
      workflow.value = workflowError({
        message: parsedEditor.value.error || issueMessage(editorAnalysis.value.issues[0]) || '快照无效',
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

  async function importSnapshot(file: SnapshotImportFile) {
    try {
      editorText.value = await readSnapshotImport(file)
      workflow.value = workflowSuccess('已导入快照')
    } catch (error) {
      workflow.value = workflowError(error)
    }
  }

  function exportSnapshot(): SnapshotExport | null {
    const snapshot = editorAnalysis.value.snapshot
    return snapshot ? createSnapshotExport(snapshot) : null
  }

  async function submitReview() {
    if (!version.value) return
    const result = await run('reviewing', () => api().submitReview(version.value!.id, version.value!.revision), '已提交审核')
    if (result) setVersion(result)
  }

  async function approveReview() {
    if (!version.value) return
    const result = await run('reviewing', () => api().approveReview(version.value!.id, version.value!.revision), '审核已通过')
    if (result) setVersion(result)
  }

  async function rejectReview() {
    if (!version.value || !rejectReason.value.trim()) {
      workflow.value = workflowError({ message: '请填写退回原因' })
      return
    }
    const result = await run('reviewing', () => api().rejectReview(version.value!.id, version.value!.revision, rejectReason.value.trim()), '已退回草稿')
    if (result) setVersion(result)
  }

  async function publish() {
    if (!version.value) return
    const current = version.value
    const key = operationKeys.get('publish', current.id)
    const result = await run('publishing', () => api().publish(current.id, key), '发布任务已创建')
    if (result) setPublishJob('publish', result)
  }

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

  async function rollback() {
    if (!version.value) return
    const current = version.value
    const key = operationKeys.get('rollback', current.id)
    const result = await run('publishing', () => api().rollback(current.id, key), '回滚任务已创建')
    if (result) setPublishJob('rollback', result)
  }

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

function issueMessage(issue: { path: string; code: string } | undefined): string {
  if (!issue) return ''
  if (issue.code === 'invalid-json') return 'JSON 格式无效'
  if (issue.code === 'object-root') return '快照必须是 JSON 对象'
  return `快照无效：${issue.path}`
}
