import { computed, ref, type Ref } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import { analyzeSnapshotText, type SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { applyMusicDraft, createMusicDraft, type MusicDraft, type MusicErrorCode } from '../utils/music-workbench'

type PendingMusicAction = { kind: 'select'; id: string | null } | { kind: 'close' }

/**
 * 保留未应用音乐表单，并在显式确认切换或完整校验成功后改变编辑上下文。
 * 列表复用防抖快照；有副作用的打开和应用始终读取当前文本。
 */
export function useMusicWorkspace(options: {
  editorText: Ref<string>
  busy: Readonly<Ref<boolean>>
  snapshot: Readonly<Ref<YujianContentSnapshot | null>>
}) {
  const draft = ref<MusicDraft | null>(null)
  const baseline = ref('')
  const initialDraft = ref('')
  const error = ref<MusicErrorCode | null>(null)
  const issues = ref<readonly SnapshotEditorIssue[]>([])
  const pending = ref<PendingMusicAction | null>(null)
  const dirty = computed(() => Boolean(draft.value && (draft.value.originalId === null || JSON.stringify(draft.value) !== initialDraft.value)))
  const stale = computed(() => Boolean(draft.value && options.editorText.value !== baseline.value))
  const releases = computed(() => options.snapshot.value?.releases ?? [])

  /** 同步检查当前文本，再生成独立的临时表单和应用基线。 */
  function open(id: string | null): boolean {
    const snapshot = analyzeSnapshotText(options.editorText.value).snapshot
    if (!snapshot) {
      error.value = 'invalid-snapshot'
      return false
    }
    const next = createMusicDraft(snapshot, id)
    if (!next) {
      error.value = 'missing-release'
      return false
    }
    draft.value = next
    baseline.value = options.editorText.value
    initialDraft.value = JSON.stringify(next)
    error.value = null
    issues.value = []
    return true
  }

  /** 在未应用输入存在时暂存切换意图，等待继续编辑或明确放弃。 */
  function select(id: string | null): boolean {
    if (options.busy.value) return false
    if (dirty.value) {
      pending.value = { kind: 'select', id }
      return false
    }
    return open(id)
  }

  /** 只在文本基线仍一致且整个事务有效时更新唯一快照源。 */
  function apply(): boolean {
    if (options.busy.value || !draft.value) return false
    const result = applyMusicDraft(options.editorText.value, baseline.value, draft.value)
    error.value = result.error
    issues.value = result.issues
    if (result.error) return false
    const id = draft.value.release.id
    options.editorText.value = result.text
    return open(id)
  }

  /** 清理临时状态；调用方只在明确放弃或替换快照后使用。 */
  function reset() {
    draft.value = null
    initialDraft.value = ''
    baseline.value = ''
    pending.value = null
    error.value = null
    issues.value = []
  }

  /** 关闭干净表单；存在输入时先请求明确的放弃决定。 */
  function close() {
    if (options.busy.value) return
    if (dirty.value) pending.value = { kind: 'close' }
    else reset()
  }

  /** 用户选择继续编辑时，仅取消待执行意图而不修改任何输入。 */
  function keepEditing() {
    pending.value = null
  }

  /** 执行已明确确认的切换，忙碌时保持待办和表单不变。 */
  function discardPending() {
    if (options.busy.value || !pending.value) return
    const action = pending.value
    if (action.kind === 'close') reset()
    else if (open(action.id)) pending.value = null
  }

  return { draft, error, issues, pending, dirty, stale, releases, select, apply, reset, close, keepEditing, discardPending }
}
