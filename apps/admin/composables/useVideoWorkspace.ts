import { computed, ref, type Ref } from 'vue'
import { analyzeSnapshotText, type SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { applyVideoDraft, createVideoDraft, type VideoDraft, type VideoErrorCode } from '../utils/video-workbench'

/** 管理影像草稿与待确认操作。 */
export function useVideoWorkspace(options: { editorText: Ref<string>; busy: Readonly<Ref<boolean>> }) {
  const draft = ref<VideoDraft | null>(null)
  const baseline = ref('')
  const initial = ref('')
  const dirty = computed(() => Boolean(draft.value && JSON.stringify(draft.value) !== initial.value))
  const stale = computed(() => Boolean(draft.value && options.editorText.value !== baseline.value))
  const error = ref<VideoErrorCode | null>(null)
  const issues = ref<readonly SnapshotEditorIssue[]>([])
  const pending = ref<'open' | 'close' | null>(null)
  /** 同步读取当前原文；失败保留旧草稿和待确认意图。 */
  function readCurrent(): boolean {
    const analysis = analyzeSnapshotText(options.editorText.value)
    if (!analysis.snapshot) {
      error.value = 'invalid-snapshot'
      issues.value = analysis.issues
      return false
    }
    draft.value = createVideoDraft(analysis.snapshot)
    baseline.value = options.editorText.value
    initial.value = JSON.stringify(draft.value)
    error.value = null
    issues.value = []
    pending.value = null
    return true
  }
  /** 脏表单先记录重载意图，不立即丢弃输入。 */
  function open(): boolean {
    if (options.busy.value) return false
    if (dirty.value) {
      pending.value = 'open'
      return false
    }
    return readCurrent()
  }
  /** 原子应用并刷新干净基线；校验失败保留输入。 */
  function apply(): boolean {
    if (options.busy.value || !draft.value) return false
    const result = applyVideoDraft(options.editorText.value, baseline.value, draft.value)
    error.value = result.error
    issues.value = result.issues
    if (result.error) return false
    options.editorText.value = result.text
    return readCurrent()
  }
  /** 替换成功或确认关闭后清理上下文，不修改 JSON。 */
  function reset() {
    draft.value = null
    baseline.value = ''
    initial.value = ''
    error.value = null
    issues.value = []
    pending.value = null
  }
  /** 干净时关闭，脏输入先等待确认。 */
  function close() {
    if (options.busy.value) return
    if (dirty.value) pending.value = 'close'
    else reset()
  }
  /** 取消待确认操作，保留输入。 */
  function keepEditing() { pending.value = null }
  /** 仅执行明确确认的操作，忙碌或读取失败时保留待办。 */
  function discardPending() {
    if (options.busy.value || !pending.value) return
    if (pending.value === 'close') reset()
    else readCurrent()
  }
  return { draft, dirty, stale, error, issues, pending, open, apply, reset, close, keepEditing, discardPending }
}
