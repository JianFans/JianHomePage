<script setup lang="ts">
import { useHead } from '#imports'
import { ArrowRight, Check, Download, ExternalLink, Eye, FileUp, Languages, LoaderCircle, LockKeyhole, Menu, MoreHorizontal, Save } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import '../assets/css/main.css'
import '../assets/css/studio.css'
import StudioContent from '../components/StudioContent.vue'
import HomepagePreview from '../components/HomepagePreview.vue'
import SnapshotInsights from '../components/SnapshotInsights.vue'
import StudioNavigation from '../components/StudioNavigation.vue'
import StudioVersionBar from '../components/StudioVersionBar.vue'
import StudioVersionWorkflow from '../components/StudioVersionWorkflow.vue'
import StudioDialog from '../components/StudioDialog.vue'
import ToastViewport from '../components/ToastViewport.vue'
import { useStudioWorkspace } from '../composables/useStudioWorkspace'
import { persistAdminLocale, resolveAdminLocale, type AdminLocale } from '../utils/admin-locale'
import { studioVersionStatus, type StudioModule } from '../utils/studio-copy'

const locale = ref<AdminLocale>('zh-CN')
const studio = reactive(useStudioWorkspace(locale))
const { workspace, session, toasts } = studio
const copy = computed(() => studio.copy)
const collapsed = ref(false)
const uiMode = ref<'' | 'tools' | 'preview' | 'review' | 'publish' | 'rollback' | 'navigation'>('')
const cloneName = ref('')
const snapshotFileInput = ref<HTMLInputElement | null>(null)
const modal = computed(() => studio.pending ? 'leave' : studio.cloneRequested ? 'clone' : uiMode.value)
const modalTitle = computed(() => ({ leave: copy.value.leaveTitle, clone: copy.value.clone, tools: copy.value.tools, preview: copy.value.preview,
  review: copy.value.versions, publish: copy.value.publish, rollback: copy.value.rollback, navigation: copy.value.studio, '': '' })[modal.value])
const versionName = computed(() => workspace.version?.name || workspace.version?.id || (workspace.editorAnalysis.snapshot ? copy.value.initialDraft : copy.value.noVersion))
const textCurrent = computed(() => workspace.editorText === workspace.analyzedText)
const validationLabel = computed(() => workspace.editorAnalysis.issues.length ? `${workspace.editorAnalysis.issues.length} · ${copy.value.invalid}` : copy.value.valid)
watch(() => studio.cloneRequested, (requested) => { if (requested) cloneName.value = `${versionName.value} · ${copy.value.newName}` })
watch(() => session.authenticated, (active) => {
  if (!active) { uiMode.value = ''; studio.pending = null; studio.cloneRequested = false }
})

useHead(() => ({ htmlAttrs: { lang: locale.value } }))

/** 切换语言只改变管理端偏好，不改变路由或内容语言。 */
function toggleLocale() {
  locale.value = locale.value === 'en' ? 'zh-CN' : 'en'
  persistAdminLocale(locale.value)
}
/** 手机导航关闭后再请求模块切换，未保存保护使用同一条路径。 */
async function navigate(module: StudioModule) { uiMode.value = ''; await studio.request({ kind: 'module', module }) }
/** 忙碌时不关闭确认，以免隐藏仍在执行的写操作。 */
function closeModal() {
  if (workspace.busy) return
  studio.pending = null
  studio.cloneRequested = false
  uiMode.value = ''
}
/** 文件交接必须等待异步读取，清空原输入以支持重选同名文件。 */
async function handleSnapshotFile(event: Event) {
  const input = event.currentTarget as HTMLInputElement
  const file = input.files?.[0]
  if (file && !studio.locked) await studio.request({ kind: 'import', file })
  input.value = ''
}
/** 先应用当前表单供预览使用；预览只收到快照，不带任何认证信息。 */
function preview() { if (studio.applyForPreview()) uiMode.value = 'preview' }
/** 脱离文档的下载链接交给浏览器后，再释放临时对象地址。 */
function downloadSnapshot() {
  if (workspace.busy || !studio.applyForPreview()) return
  const exported = workspace.exportSnapshot()
  if (!exported) return
  const url = URL.createObjectURL(new Blob([exported.contents], { type: exported.mimeType }))
  try {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = exported.filename
    anchor.click()
  } finally { setTimeout(() => URL.revokeObjectURL(url), 0) }
}
/** 发布和回滚必须通过明确确认；普通版本选择不会调用这些操作。 */
async function confirmRelease() {
  if (uiMode.value === 'publish' && studio.canPublish && !studio.unsaved) await workspace.publish()
  else if (uiMode.value === 'rollback' && studio.canPublish && !studio.unsaved) await workspace.rollback()
  if (workspace.workflow.status === 'success') uiMode.value = ''
}
/** 标准离开保护覆盖尚未应用的表单与已经应用但未保存的快照。 */
function beforeUnload(event: BeforeUnloadEvent) { if (studio.unsaved || studio.assetDirty || studio.assetBusy) { event.preventDefault(); event.returnValue = '' } }
onMounted(() => {
  locale.value = resolveAdminLocale(undefined, navigator.language)
  window.addEventListener('beforeunload', beforeUnload)
  void studio.initialize()
})
onBeforeUnmount(() => window.removeEventListener('beforeunload', beforeUnload))
</script>

<template>
  <div
    class="studio-root"
    :class="{ 'studio-root--collapsed': collapsed }"
  >
    <div :inert="Boolean(modal)">
      <div
        v-if="studio.initializing"
        class="studio-loading"
        data-testid="studio-loading"
        role="status"
      >
        <LoaderCircle
          :size="24"
          aria-hidden="true"
        /><span>{{ copy.loading }}</span>
      </div>
      <main
        v-else-if="!session.authenticated"
        class="studio-login-page"
      >
        <div
          class="studio-login-art"
          aria-hidden="true"
        >
          <span>遇</span><div /><small>YUJIAN.ME</small>
        </div>
        <section class="studio-login-card">
          <p class="studio-kicker">
            YUJIAN.ME / STUDIO
          </p><h1>{{ copy.brand }}</h1><p class="studio-muted">
            {{ copy.studio }}
          </p>
          <div class="studio-login-rule" />
          <LockKeyhole
            :size="24"
            aria-hidden="true"
          />
          <p class="studio-muted">
            {{ session.expired ? copy.expired : session.loginAvailable ? copy.loginHint : copy.unavailable }}
          </p>
          <a
            v-if="session.loginAvailable"
            class="button button--primary"
            :href="session.loginUrl"
            data-testid="studio-login"
          >{{ copy.login }}<ArrowRight
            :size="17"
            aria-hidden="true"
          /></a>
          <button
            v-if="session.error || !session.loginAvailable"
            type="button"
            class="button"
            :disabled="session.busy"
            @click="studio.initialize"
          >
            {{ copy.retry }}
          </button>
          <button
            v-if="session.expired && studio.unsaved"
            class="button"
            type="button"
            @click="downloadSnapshot"
          >
            <Download
              :size="16"
              aria-hidden="true"
            />{{ copy.export }}
          </button>
          <button
            class="studio-locale rail-locale"
            type="button"
            :aria-label="copy.language"
            @click="toggleLocale"
          >
            <Languages
              :size="17"
              aria-hidden="true"
            /><span>{{ locale === 'en' ? 'EN' : '中文' }}</span>
          </button>
        </section>
      </main>
      <div
        v-if="session.authenticated || session.expired"
        v-show="session.authenticated && !studio.initializing"
        :inert="!session.authenticated || session.busy"
        class="studio-shell"
        data-testid="studio-shell"
      >
        <StudioNavigation
          :active="studio.module"
          :locale="locale"
          :name="session.principal?.name || session.principal?.subject || ''"
          :picture="session.principal?.picture"
          :collapsed="collapsed"
          :busy="workspace.busy"
          :logout-busy="studio.assetBusy || session.busy"
          @navigate="navigate"
          @logout="studio.request({ kind: 'logout' })"
          @collapse="collapsed = !collapsed"
        />
        <div class="studio-main">
          <header class="studio-topbar">
            <button
              class="icon-tool studio-menu"
              type="button"
              :aria-label="copy.menu"
              @click="uiMode = 'navigation'"
            >
              <Menu
                :size="20"
                aria-hidden="true"
              />
            </button>
            <div class="studio-breadcrumb">
              <span>{{ copy.studio }}</span><span aria-hidden="true">/</span><strong>{{ copy.modules[studio.module] }}</strong>
            </div>
            <div class="studio-topbar-actions">
              <a
                class="icon-tool studio-public-link"
                href="https://yujian.me"
                target="_blank"
                rel="noopener noreferrer"
                aria-label="yujian.me"
              ><ExternalLink
                :size="17"
                aria-hidden="true"
              /></a>
              <button
                type="button"
                class="icon-tool rail-locale"
                :aria-label="copy.language"
                :title="copy.language"
                @click="toggleLocale"
              >
                <Languages
                  :size="18"
                  aria-hidden="true"
                />
              </button>
              <button
                type="button"
                class="button studio-preview"
                :disabled="workspace.busy || !workspace.editorAnalysis.snapshot"
                data-testid="studio-preview"
                :aria-label="copy.preview"
                @click="preview"
              >
                <Eye
                  :size="16"
                  aria-hidden="true"
                /><span>{{ copy.preview }}</span>
              </button>
              <button
                type="button"
                class="button button--primary studio-save"
                data-testid="snapshot-save"
                :disabled="studio.locked"
                :aria-label="copy.save"
                @click="studio.save"
              >
                <LoaderCircle
                  v-if="workspace.busy"
                  :size="16"
                  class="studio-spinner"
                  aria-hidden="true"
                /><Save
                  v-else
                  :size="16"
                  aria-hidden="true"
                /><span>{{ copy.save }}</span>
              </button>
            </div>
          </header>
          <StudioVersionBar
            :version="workspace.version"
            :versions="workspace.versions"
            :locale="locale"
            :busy="workspace.busy"
            :can-clone="studio.canEdit"
            @select="id => workspace.version?.id !== id && studio.request({ kind: 'version', id })"
            @clone="studio.request({ kind: 'clone' })"
            @history="navigate('history')"
          />
          <section
            class="studio-version-context"
            data-testid="studio-version-context"
            :aria-label="copy.current"
          >
            <div class="studio-version-info">
              <p class="studio-kicker">
                {{ copy.current }}
              </p>
              <strong data-testid="studio-version-name">{{ versionName }}</strong>
              <div class="studio-meta">
                <span
                  v-if="workspace.version"
                  class="studio-status"
                  :data-status="studioVersionStatus(workspace.version)"
                >{{ copy.statuses[studioVersionStatus(workspace.version)] }}</span>
                <span>{{ studio.locked && !workspace.busy ? copy.readonly : studio.unsaved ? copy.unsaved : copy.saved }}</span>
                <span v-if="workspace.version">{{ copy.revision }} {{ workspace.version.revision }}</span>
              </div>
            </div>
            <StudioVersionWorkflow
              :studio="studio"
              @workflow="mode => uiMode = mode"
            />
          </section>
          <main
            class="studio-content"
            data-studio-content
          >
            <div class="studio-page-heading">
              <div>
                <h1>
                  {{ copy.modules[studio.module] }}
                </h1>
              </div>
              <button
                type="button"
                class="icon-tool"
                data-testid="studio-tools"
                :aria-label="copy.tools"
                :title="copy.tools"
                :disabled="workspace.busy || studio.formDirty"
                @click="uiMode = 'tools'"
              >
                <MoreHorizontal
                  :size="20"
                  aria-hidden="true"
                />
              </button>
            </div>
            <StudioContent
              :studio="studio"
              :locale="locale"
              @navigate="navigate"
              @asset-busy="value => studio.assetBusy = value"
              @asset-dirty="value => studio.assetDirty = value"
            />
          </main>
        </div>
      </div>
    </div>
    <StudioDialog
      :open="Boolean(modal)"
      :content-key="modal"
      :title="modalTitle"
      :close-label="copy.close"
      :busy="workspace.busy"
      :wide="modal === 'tools' || modal === 'preview'"
      @close="closeModal"
    >
      <div
        v-if="modal === 'leave'"
        data-testid="leave-dialog"
      >
        <p class="studio-muted">
          {{ studio.pending?.kind === 'logout' && studio.assetDirty ? copy.assetLeaveHint : copy.leaveHint }}
        </p><div class="studio-dialog-actions">
          <button
            class="button"
            type="button"
            data-testid="leave-keep"
            :disabled="workspace.busy"
            @click="closeModal"
          >
            {{ copy.keep }}
          </button><button
            class="button button--danger"
            type="button"
            data-testid="leave-discard"
            :disabled="workspace.busy"
            @click="studio.discardPending"
          >
            {{ copy.discard }}
          </button><button
            class="button button--primary"
            type="button"
            data-testid="leave-save"
            :disabled="studio.locked || (studio.pending?.kind === 'logout' && studio.assetDirty)"
            @click="studio.savePending"
          >
            {{ copy.saveContinue }}
          </button>
        </div>
      </div>
      <form
        v-else-if="modal === 'clone'"
        @submit.prevent="studio.clone(cloneName)"
      >
        <label class="studio-field"><span>{{ copy.cloneName }}</span><input
          v-model="cloneName"
          autofocus
          required
          maxlength="120"
          data-testid="clone-name"
          :disabled="workspace.busy"
        ></label><p class="studio-muted">
          {{ copy.source }} · {{ versionName }}
        </p><div class="studio-dialog-actions">
          <button
            class="button button--primary"
            type="submit"
            data-testid="clone-confirm"
            :disabled="workspace.busy || !cloneName.trim()"
          >
            {{ copy.create }}
          </button>
        </div>
      </form>
      <div
        v-else-if="modal === 'tools'"
        class="studio-tools"
      >
        <div class="studio-tools-bar">
          <span
            data-testid="snapshot-validation"
            :class="workspace.editorAnalysis.issues.length ? 'studio-muted' : 'studio-valid'"
          >{{ validationLabel }}</span><button
            class="button"
            type="button"
            data-testid="snapshot-import"
            :disabled="studio.locked"
            @click="snapshotFileInput?.click()"
          >
            <FileUp
              :size="16"
              aria-hidden="true"
            />{{ copy.import }}
          </button><button
            class="button"
            type="button"
            data-testid="snapshot-export"
            :disabled="workspace.busy || !workspace.editorAnalysis.snapshot"
            @click="downloadSnapshot"
          >
            <Download
              :size="16"
              aria-hidden="true"
            />{{ copy.export }}
          </button>
        </div>
        <input
          ref="snapshotFileInput"
          type="file"
          accept="application/json,.json"
          class="sr-only"
          tabindex="-1"
          :aria-label="copy.import"
          data-testid="snapshot-file-input"
          :disabled="studio.locked"
          @change="handleSnapshotFile"
        >
        <textarea
          v-model="workspace.editorText"
          class="json-editor"
          :aria-label="copy.editor"
          :readonly="studio.locked"
          spellcheck="false"
        />
        <p
          v-if="workspace.parsedEditor.error"
          class="field-error"
          role="status"
        >
          {{ workspace.parsedEditor.error }}
        </p>
        <SnapshotInsights
          :analysis="workspace.editorAnalysis"
          :locale="locale"
        />
      </div>
      <HomepagePreview
        v-else-if="modal === 'preview'"
        :snapshot="workspace.editorAnalysis.snapshot"
        :text-is-current="textCurrent"
        :text-revision="studio.textRevision"
        :form-dirty="studio.formDirty"
        :locale="locale"
        auto-open
      />
      <div
        v-else-if="modal === 'review'"
        class="studio-review"
      >
        <button
          class="button button--primary"
          type="button"
          :disabled="!workspace.canApprove || !studio.canReview"
          @click="workspace.approveReview"
        >
          <Check
            :size="16"
            aria-hidden="true"
          />{{ copy.approve }}
        </button><label class="studio-field"><span>{{ copy.reason }}</span><input
          v-model="workspace.rejectReason"
          :disabled="!workspace.canApprove || !studio.canReview"
        ></label><button
          class="button button--danger"
          type="button"
          :disabled="!workspace.canReject || !studio.canReview"
          @click="workspace.rejectReview"
        >
          {{ copy.reject }}
        </button>
      </div>
      <div v-else-if="modal === 'publish' || modal === 'rollback'">
        <p class="studio-muted">
          {{ copy.publishingHint }}
        </p><div class="studio-dialog-actions">
          <button
            class="button button--primary"
            type="button"
            :disabled="workspace.busy || !studio.canPublish"
            @click="confirmRelease"
          >
            {{ copy.confirm }}
          </button>
        </div>
      </div>
      <StudioNavigation
        v-else-if="modal === 'navigation'"
        :active="studio.module"
        :locale="locale"
        :name="session.principal?.name || session.principal?.subject || ''"
        :picture="session.principal?.picture"
        drawer
        :busy="workspace.busy"
        :logout-busy="studio.assetBusy || session.busy"
        @navigate="navigate"
        @logout="uiMode = ''; studio.request({ kind: 'logout' })"
      />
      <template #messages>
        <ToastViewport
          :items="toasts.items"
          :locale="locale"
          @dismiss="toasts.dismiss"
          @pause="toasts.pause"
          @resume="toasts.resume"
          @pause-all="toasts.pauseAll"
          @resume-all="toasts.resumeAll"
          @visibility="hidden => hidden ? toasts.pauseAll('hidden') : toasts.resumeAll('hidden')"
        />
      </template>
    </StudioDialog>
  </div>
</template>
