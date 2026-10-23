import { computed, ref, type Ref } from 'vue'
import { analyzeSnapshotText, type SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { applySiteDraft, createSiteDraft, type SiteDraft, type SiteError } from '../utils/site-workbench'

/** 站点资料复用工作区的未保存、过期与完整快照保存规则。 */
export function useSiteWorkspace(options: { editorText: Ref<string>; busy: Readonly<Ref<boolean>> }) {
  const draft = ref<SiteDraft | null>(null)
  const baseline = ref('')
  const initial = ref('')
  const error = ref<SiteError | null>(null)
  const issues = ref<readonly SnapshotEditorIssue[]>([])
  const dirty = computed(() => Boolean(draft.value && JSON.stringify(draft.value) !== initial.value))
  const stale = computed(() => Boolean(draft.value && options.editorText.value !== baseline.value))
  /** 模块打开时同步读取，避免防抖中的旧资料覆盖当前文本。 */
  function open() {
    if (options.busy.value || dirty.value) return false
    const analysis = analyzeSnapshotText(options.editorText.value)
    if (!analysis.snapshot) { error.value = 'invalid-snapshot'; issues.value = analysis.issues; return false }
    draft.value = createSiteDraft(analysis.snapshot)
    baseline.value = options.editorText.value
    initial.value = JSON.stringify(draft.value)
    error.value = null
    issues.value = []
    return true
  }
  /** 完整成功后再更新编辑器，任一错误保留草稿。 */
  function apply() {
    if (options.busy.value || !draft.value) return false
    const result = applySiteDraft(options.editorText.value, baseline.value, draft.value)
    error.value = result.error
    issues.value = result.issues
    if (result.error) return false
    options.editorText.value = result.text
    baseline.value = result.text
    initial.value = JSON.stringify(draft.value)
    return true
  }
  /** 切换成功后清理表单，不清理已应用的 JSON。 */
  function reset() { draft.value = null; baseline.value = ''; initial.value = ''; error.value = null; issues.value = [] }
  return { draft, dirty, stale, error, issues, open, apply, reset }
}
