import { computed, ref, type Ref } from 'vue'
import { analyzeSnapshotText, type SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { applyHomepageDraft, createHomepageDraft, type HomepageDraft, type HomepageErrorCode } from '../utils/homepage-workbench'

/** 保留首页临时副本，只有基线一致且完整校验成功才更新唯一 JSON 内容源。 */
export function useHomepageWorkspace(options: { editorText: Ref<string>; busy: Readonly<Ref<boolean>> }) {
  const draft = ref<HomepageDraft | null>(null)
  const baseline = ref('')
  const initial = ref('')
  const error = ref<HomepageErrorCode | null>(null)
  const issues = ref<readonly SnapshotEditorIssue[]>([])
  const pending = ref<'open' | 'close' | null>(null)
  const dirty = computed(() => Boolean(draft.value && JSON.stringify(draft.value) !== initial.value))
  const stale = computed(() => Boolean(draft.value && options.editorText.value !== baseline.value))

  /** 同步读取当前文本，失败时保留已有表单与待确认操作。 */
  function readCurrent(): boolean {
    const analysis = analyzeSnapshotText(options.editorText.value)
    if (!analysis.snapshot) {
      error.value = 'invalid-snapshot'
      issues.value = analysis.issues
      return false
    }
    draft.value = createHomepageDraft(analysis.snapshot)
    baseline.value = options.editorText.value
    initial.value = JSON.stringify(draft.value)
    error.value = null
    issues.value = []
    pending.value = null
    return true
  }

  /** 打开或重载草稿；脏表单只记录意图，不立即丢弃输入。 */
  function open(): boolean {
    if (options.busy.value) return false
    if (dirty.value) {
      pending.value = 'open'
      return false
    }
    return readCurrent()
  }

  /** 原子应用宣传位与编排，失败时保留输入和精确诊断。 */
  function apply(): boolean {
    if (options.busy.value || !draft.value) return false
    const result = applyHomepageDraft(options.editorText.value, baseline.value, draft.value)
    error.value = result.error
    issues.value = result.issues
    if (result.error) return false
    options.editorText.value = result.text
    return readCurrent()
  }

  /** 在已确认的替换成功后清理临时上下文，不改变 JSON。 */
  function reset() {
    draft.value = null
    baseline.value = ''
    initial.value = ''
    error.value = null
    issues.value = []
    pending.value = null
  }

  /** 干净时直接关闭，存在未应用输入时先请求确认。 */
  function close() {
    if (options.busy.value) return
    if (dirty.value) pending.value = 'close'
    else reset()
  }

  /** 取消待确认意图，保留所有编辑输入。 */
  function keepEditing() {
    pending.value = null
  }

  /** 仅执行用户明确确认的关闭或重载，busy 时保留待办。 */
  function discardPending() {
    if (options.busy.value || !pending.value) return
    if (pending.value === 'close') reset()
    else readCurrent()
  }

  return { draft, dirty, stale, error, issues, pending, open, apply, reset, close, keepEditing, discardPending }
}
