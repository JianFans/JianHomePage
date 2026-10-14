import { computed, nextTick, reactive, ref, shallowRef, toRef, watch, type Ref, type UnwrapNestedRefs } from 'vue'
import { useAdminSession } from './useAdminSession'
import { useAdminWorkspace } from './useAdminWorkspace'
import { useMusicWorkspace } from './useMusicWorkspace'
import { useHomepageWorkspace } from './useHomepageWorkspace'
import { useVideoWorkspace } from './useVideoWorkspace'
import { useEventWorkspace } from './useEventWorkspace'
import { useMomentWorkspace } from './useMomentWorkspace'
import { useToasts } from './useToasts'
import { analyzeSnapshotText, type SnapshotImportFile } from '../utils/snapshot-workbench'
import { studioCopy, type StudioModule } from '../utils/studio-copy'
import type { AdminLocale } from '../utils/admin-locale'
import { musicCopy } from '../utils/music-copy'
import { homepageCopy } from '../utils/homepage-copy'
import { videoCopy } from '../utils/video-copy'
import { eventCopy } from '../utils/event-copy'
import { momentCopy } from '../utils/moment-copy'
import { focusSnapshotIssue } from '../utils/issue-accessibility'

type StudioAction = { kind: 'module'; module: StudioModule } | { kind: 'version'; id: string } | { kind: 'import'; file: SnapshotImportFile } | { kind: 'logout' } | { kind: 'clone' } | { kind: 'release'; id: string | null } | { kind: 'close-release' }

export type StudioWorkspace = UnwrapNestedRefs<ReturnType<typeof useStudioWorkspace>>

/** 组织整站版本上下文和单一活动表单，普通编辑不需要理解 JSON 应用步骤。 */
export function useStudioWorkspace(locale: Ref<AdminLocale>) {
  const session = reactive(useAdminSession())
  const workspace = reactive(useAdminWorkspace(locale, { csrfToken: toRef(session, 'csrfToken'), active: toRef(session, 'authenticated'), onUnauthorized: session.expire }))
  const toasts = reactive(useToasts())
  const module = ref<StudioModule>('overview')
  const copy = computed(() => studioCopy(locale.value))
  const canEdit = computed(() => session.authenticated && Boolean(session.principal?.roles.some(role => role === 'admin' || role === 'editor')))
  const canReview = computed(() => session.authenticated && Boolean(session.principal?.roles.some(role => role === 'admin' || role === 'reviewer')))
  const canPublish = computed(() => session.authenticated && Boolean(session.principal?.roles.some(role => role === 'admin' || role === 'publisher')))
  const locked = computed(() => workspace.busy || !workspace.editable || !canEdit.value)
  const options = { editorText: toRef(workspace, 'editorText'), busy: toRef(workspace, 'busy') }
  const music = reactive(useMusicWorkspace({ ...options, snapshot: computed(() => workspace.editorAnalysis.snapshot) }))
  const homepage = reactive(useHomepageWorkspace(options))
  const video = reactive(useVideoWorkspace(options))
  const event = reactive(useEventWorkspace(options))
  const moment = reactive(useMomentWorkspace(options))
  const forms = { music, homepage, video, event, moment }
  const formDirty = computed(() => Object.values(forms).some(form => form.dirty))
  const unsaved = computed(() => workspace.dirty || formDirty.value)
  const pending = shallowRef<StudioAction | null>(null)
  const initializing = ref(false)
  const cloneRequested = ref(false)
  const assetBusy = ref(false)
  const assetDirty = ref(false)
  const textRevision = ref(0)
  watch(() => workspace.editorText, () => { textRevision.value++ }, { flush: 'sync' })

  watch(() => workspace.workflow, (state) => {
    if ((state.status === 'success' || state.status === 'error') && state.message) {
      toasts.notify(state.requestId ? `${state.message} · ${state.requestId}` : state.message, state.status === 'error' ? 'error' : 'success')
    }
  })
  watch(() => session.error, (error) => {
    if (error) toasts.notify(error === 'logout-failed' ? copy.value.logoutError : copy.value.sessionError, 'error')
  })
  watch(() => session.expired, (expired) => { if (expired) toasts.notify(copy.value.expired, 'warning') })
  watch(() => workspace.listError, error => { if (error) toasts.notify(copy.value.listError, 'error') })

  /** 只在干净上下文中重建表单，保留失败请求时的临时输入。 */
  function resetForms() { for (const form of Object.values(forms)) form.reset() }

  /** 提示包含当前模块、稳定错误含义及首个契约路径，不把失败插入内容布局。 */
  function reportFormFailure() {
    const active = module.value as keyof typeof forms
    const form = forms[active]
    const copies = { music: musicCopy(locale.value), homepage: homepageCopy(locale.value), video: videoCopy(locale.value), event: eventCopy(locale.value), moment: momentCopy(locale.value) }
    const errors = copies[active]?.errors as Record<string, string> | undefined
    const message = (form?.error && errors?.[form.error]) || copy.value.formInvalid
    const path = form?.issues[0]?.path
    toasts.notify(`${copy.value.modules[module.value]} · ${message}${path ? ` · ${path}` : ''}`, 'error')
  }

  /** 历史表单允许读取，但所有修改控件通过 locked 禁用。 */
  function openModule() {
    const active = module.value
    const readableForms = { homepage, video, event, moment }
    if (!(active in readableForms)) return
    const form = readableForms[active as keyof typeof readableForms]
    if (!workspace.busy) form.open()
  }

  /** 保存先同步应用当前表单，再提交完整快照；失败保留可重试的内容与基线。 */
  async function save(): Promise<boolean> {
    if (locked.value) return false
    const active = module.value in forms ? forms[module.value as keyof typeof forms] : null
    if (active?.dirty && !active.apply()) {
      reportFormFailure()
      await nextTick()
      const root = document.querySelector<HTMLElement>('[data-studio-content]')
      if (root && !await focusSnapshotIssue(root, active.issues)) {
        root.querySelector<HTMLElement>('[data-module] input:invalid, [data-module] [aria-invalid="true"]')?.focus()
      }
      return false
    }
    const selectedRelease = music.draft?.release.id
    await workspace.saveDraft()
    if (workspace.workflow.status !== 'success' || !session.authenticated) return false
    resetForms()
    if (module.value === 'music' && selectedRelease) music.select(selectedRelease)
    else openModule()
    return true
  }

  /** 进入模块时只清理表单临时态，已应用但未保存的快照留在原版本。 */
  async function execute(action: StudioAction) {
    if (workspace.busy) return
    if (action.kind === 'module') {
      resetForms()
      module.value = action.module
      openModule()
    } else if (action.kind === 'version') {
      workspace.versionId = action.id
      await workspace.loadVersion()
      if (workspace.workflow.status === 'success') { resetForms(); openModule() }
    } else if (action.kind === 'import') {
      await workspace.importSnapshot(action.file, locale.value)
      if (workspace.workflow.status === 'success') { resetForms(); openModule() }
    } else if (action.kind === 'release') {
      if (!music.select(action.id) && music.error) reportFormFailure()
    } else if (action.kind === 'close-release') music.reset()
    else if (action.kind === 'logout') {
      if (await session.logout()) { resetForms(); workspace.clear(); assetDirty.value = false; assetBusy.value = false; module.value = 'overview' }
    } else cloneRequested.value = true
  }

  /** 模块只保护临时表单；版本、导入和退出保护整份未保存快照。 */
  async function request(action: StudioAction) {
    if (workspace.busy || initializing.value) return
    if (action.kind === 'logout' && assetBusy.value) { toasts.notify(copy.value.uploadHint, 'warning'); return }
    const needsConfirmation = action.kind === 'module' || action.kind === 'release' || action.kind === 'close-release' ? formDirty.value : unsaved.value || (action.kind === 'logout' && assetDirty.value)
    if (action.kind === 'module' && action.module === module.value) return
    if (needsConfirmation) pending.value = action
    else await execute(action)
  }

  /** 放弃当前操作涉及的输入；载入失败仍保留原内容，因此先执行替换再清理。 */
  async function discardPending() {
    const action = pending.value
    if (!action || workspace.busy) return
    pending.value = null
    if (action.kind === 'clone') {
      // Clone reads the saved source. Keep unsaved content until it succeeds.
      cloneRequested.value = true
    } else {
      if (action.kind === 'release' || action.kind === 'close-release') music.reset()
      await execute(action)
    }
  }

  /** 保存失败保持确认对话框，禁止提前切换上下文。 */
  async function savePending() {
    const action = pending.value
    if (action?.kind === 'logout' && assetDirty.value) return
    if (!action || !(await save())) return
    pending.value = null
    await execute(action)
  }

  /** 复制成功才替换上下文；失败保留源版本及所有尚未保存的输入。 */
  async function clone(name: string) {
    if (!canEdit.value || workspace.busy || !workspace.version) return false
    if (!(await workspace.cloneVersion(workspace.version.id, name))) return false
    resetForms()
    openModule()
    cloneRequested.value = false
    return true
  }

  /** 会话恢复后读取摘要并选择最近草稿，初次空库不隐式加载测试内容。 */
  async function initialize() {
    if (initializing.value) return
    initializing.value = true
    try {
      await session.restore()
      if (!session.authenticated) return
      await workspace.listVersions()
      if (!session.authenticated || workspace.listError) return
      if (workspace.version) return
      const cursors = new Set<string>()
      while (!workspace.versions.some(item => item.status === 'draft') && workspace.nextCursor) {
        if (cursors.has(workspace.nextCursor)) return
        cursors.add(workspace.nextCursor)
        await workspace.listVersions(true)
        if (!session.authenticated || workspace.listError) return
      }
      const next = workspace.versions.find(item => item.status === 'draft') || workspace.versions[0]
      if (next) { workspace.versionId = next.id; await workspace.loadVersion() }
    } finally { initializing.value = false }
  }

  /** 预览与导出使用当前文本；未应用表单应先通过统一事务应用。 */
  function applyForPreview(): boolean {
    const active = module.value in forms ? forms[module.value as keyof typeof forms] : null
    if (!active?.dirty) return Boolean(analyzeSnapshotText(workspace.editorText).snapshot)
    if (!workspace.busy && active.apply()) return true
    reportFormFailure()
    return false
  }

  return { session, workspace, toasts, module, copy, canEdit, canReview, canPublish, locked, forms, music, homepage, video, event, moment, formDirty, unsaved, assetBusy, assetDirty, pending, initializing, cloneRequested, textRevision, request, save, discardPending, savePending, clone, initialize, applyForPreview }
}
