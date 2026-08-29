<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useAdminWorkspace } from '../composables/useAdminWorkspace'
import { persistAdminLocale, resolveAdminLocale } from '../utils/admin-locale'

const workspace = reactive(useAdminWorkspace())
const locale = ref<'zh-CN' | 'en'>('zh-CN')

const copy = computed(() => locale.value === 'en'
  ? {
      brand: 'Meet Jian',
      title: 'Content workspace',
      overview: 'Overview',
      content: 'Content',
      publish: 'Publish',
      connection: 'Connection',
      api: 'API base URL',
      token: 'Session token',
      versionId: 'Version ID',
      load: 'Load',
      editor: 'Snapshot editor',
      preview: 'Preview',
      save: 'Save draft',
      submit: 'Submit review',
      approve: 'Approve',
      reject: 'Return',
      rejectReason: 'Reason',
      publishAction: 'Publish',
      refresh: 'Refresh',
      rollback: 'Rollback',
      revision: 'Revision',
      status: 'Status',
      noJob: 'No publish job',
      tokenHint: 'Kept in this tab only',
      invalidJSON: 'JSON needs an object root',
    }
  : {
      brand: '遇健我',
      title: '内容工作台',
      overview: '概览',
      content: '内容',
      publish: '发布',
      connection: '连接',
      api: 'API 地址',
      token: '会话令牌',
      versionId: '版本 ID',
      load: '载入',
      editor: '快照编辑',
      preview: '预览',
      save: '保存草稿',
      submit: '提交审核',
      approve: '通过审核',
      reject: '退回',
      rejectReason: '退回原因',
      publishAction: '发布',
      refresh: '刷新状态',
      rollback: '回滚',
      revision: '修订号',
      status: '状态',
      noJob: '暂无发布任务',
      tokenHint: '仅保存在当前标签页',
      invalidJSON: 'JSON 根节点必须是对象',
    })

const statusLabel = computed(() => workspace.version?.status || '—')
const publishStatusLabel = computed(() => workspace.publishJob?.status || copy.value.noJob)
const previewText = computed(() => workspace.parsedEditor.snapshot
  ? JSON.stringify(workspace.parsedEditor.snapshot, null, 2)
  : workspace.parsedEditor.error || copy.value.invalidJSON)

function toggleLocale() {
  locale.value = locale.value === 'zh-CN' ? 'en' : 'zh-CN'
  if (import.meta.client) persistAdminLocale(locale.value)
}

onMounted(() => {
  locale.value = resolveAdminLocale(undefined, navigator.language)
})
</script>

<template>
  <div class="admin-shell">
    <aside
      class="rail"
      aria-label="管理端导航"
    >
      <div
        class="mark"
        aria-label="遇健我"
      >
        遇
      </div>
      <nav class="rail-nav">
        <span
          class="rail-item rail-item--active"
          :title="copy.overview"
          aria-current="page"
        >⌘</span>
        <span
          class="rail-item"
          :title="copy.content"
        >◫</span>
        <span
          class="rail-item"
          :title="copy.publish"
        >↗</span>
      </nav>
      <button
        class="rail-locale"
        type="button"
        :aria-label="locale === 'zh-CN' ? '切换英文' : '切换中文'"
        @click="toggleLocale"
      >
        {{ locale === 'zh-CN' ? '中' : 'EN' }}
      </button>
    </aside>

    <main
      class="workspace"
      data-testid="admin-workspace"
    >
      <header class="topbar">
        <div>
          <p class="eyebrow">
            {{ copy.brand }}
          </p>
          <h1>{{ copy.title }}</h1>
        </div>
        <a
          class="site-link"
          href="https://yujian.me"
          target="_blank"
          rel="noopener noreferrer"
        >yujian.me ↗</a>
      </header>

      <section
        class="connection panel"
        aria-labelledby="connection-title"
      >
        <div class="panel-heading">
          <div>
            <p class="eyebrow">
              01
            </p>
            <h2 id="connection-title">
              {{ copy.connection }}
            </h2>
          </div>
          <span
            class="status-dot"
            :class="{ 'status-dot--active': Boolean(workspace.token) }"
            aria-hidden="true"
          />
        </div>
        <div class="connection-grid">
          <label>
            <span>{{ copy.api }}</span>
            <input
              v-model="workspace.apiBaseUrl"
              type="url"
              autocomplete="url"
              spellcheck="false"
            >
          </label>
          <label>
            <span>{{ copy.token }} <small>{{ copy.tokenHint }}</small></span>
            <input
              v-model="workspace.token"
              type="password"
              autocomplete="off"
            >
          </label>
          <label>
            <span>{{ copy.versionId }}</span>
            <div class="inline-control">
              <input
                v-model="workspace.versionId"
                type="text"
                autocomplete="off"
                spellcheck="false"
              >
              <button
                class="button button--quiet"
                type="button"
                :disabled="workspace.busy"
                @click="workspace.loadVersion"
              >{{ copy.load }}</button>
            </div>
          </label>
        </div>
      </section>

      <section class="workspace-grid">
        <article
          class="panel editor-panel"
          aria-labelledby="editor-title"
        >
          <div class="panel-heading">
            <div>
              <p class="eyebrow">
                02 · {{ copy.content }}
              </p>
              <h2 id="editor-title">
                {{ copy.editor }}
              </h2>
            </div>
            <div
              v-if="workspace.version"
              class="meta-stack"
            >
              <span>{{ copy.revision }} {{ workspace.version.revision }}</span>
              <span>{{ statusLabel }}</span>
            </div>
          </div>
          <textarea
            v-model="workspace.editorText"
            class="json-editor"
            aria-label="JSON 快照编辑器"
            spellcheck="false"
          />
          <p
            v-if="workspace.parsedEditor.error"
            class="field-error"
            role="alert"
          >
            {{ workspace.parsedEditor.error }}
          </p>
          <div class="action-row">
            <button
              class="button button--primary"
              type="button"
              :disabled="!workspace.canSave"
              @click="workspace.saveDraft"
            >
              {{ copy.save }}
            </button>
            <button
              class="button"
              type="button"
              :disabled="!workspace.canSubmitReview"
              @click="workspace.submitReview"
            >
              {{ copy.submit }}
            </button>
          </div>
        </article>

        <article
          class="panel preview-panel"
          aria-labelledby="preview-title"
        >
          <div class="panel-heading">
            <div>
              <p class="eyebrow">
                03
              </p>
              <h2 id="preview-title">
                {{ copy.preview }}
              </h2>
            </div>
            <span class="preview-badge">JSON</span>
          </div>
          <pre
            class="json-preview"
            data-testid="snapshot-preview"
          >{{ previewText }}</pre>
        </article>
      </section>

      <section class="workflow-grid">
        <article
          class="panel"
          aria-labelledby="review-title"
        >
          <div class="panel-heading">
            <div>
              <p class="eyebrow">
                04
              </p>
              <h2 id="review-title">
                {{ copy.submit }} / {{ copy.approve }}
              </h2>
            </div>
            <span class="status-pill">{{ statusLabel }}</span>
          </div>
          <div class="action-row action-row--wrap">
            <button
              class="button"
              type="button"
              :disabled="!workspace.canApprove"
              @click="workspace.approveReview"
            >
              {{ copy.approve }}
            </button>
            <label class="reason-field">
              <span class="sr-only">{{ copy.rejectReason }}</span>
              <input
                v-model="workspace.rejectReason"
                type="text"
                :placeholder="copy.rejectReason"
              >
            </label>
            <button
              class="button button--danger"
              type="button"
              :disabled="workspace.busy || !workspace.version || workspace.version.status !== 'in_review'"
              @click="workspace.rejectReview"
            >
              {{ copy.reject }}
            </button>
          </div>
        </article>

        <article
          class="panel"
          aria-labelledby="publish-title"
        >
          <div class="panel-heading">
            <div>
              <p class="eyebrow">
                05
              </p>
              <h2 id="publish-title">
                {{ copy.publish }}
              </h2>
            </div>
            <span
              class="status-pill"
              :class="`status-pill--${workspace.publishJob?.status || 'idle'}`"
            >{{ publishStatusLabel }}</span>
          </div>
          <div class="action-row action-row--wrap">
            <button
              class="button button--primary"
              type="button"
              :disabled="!workspace.canPublish"
              @click="workspace.publish"
            >
              {{ copy.publishAction }}
            </button>
            <button
              class="button"
              type="button"
              :disabled="workspace.busy || !workspace.publishJob"
              @click="workspace.refreshPublish"
            >
              {{ copy.refresh }}
            </button>
            <button
              class="button button--danger"
              type="button"
              :disabled="!workspace.canRollback"
              @click="workspace.rollback"
            >
              {{ copy.rollback }}
            </button>
          </div>
          <p
            v-if="workspace.publishJob?.id"
            class="job-meta"
          >
            {{ workspace.publishJob.id }} · {{ workspace.publishJob.buildId || '—' }}
          </p>
        </article>
      </section>

      <p
        v-if="workspace.workflow.message"
        class="notice"
        :class="{ 'notice--error': workspace.workflow.status === 'error' }"
        role="status"
      >
        {{ workspace.workflow.message }}
        <span v-if="workspace.workflow.requestId"> · {{ workspace.workflow.requestId }}</span>
      </p>
    </main>
  </div>
</template>

<style scoped>
.admin-shell { min-height: 100vh; display: grid; grid-template-columns: 4.5rem 1fr; background: var(--bg); }
.rail { border-right: 1px solid var(--border); display: flex; flex-direction: column; align-items: center; padding: 1.25rem .75rem; gap: 2rem; }
.mark { width: 2.5rem; height: 2.5rem; display: grid; place-items: center; border: 1px solid var(--border); color: var(--accent); letter-spacing: .08em; }
.rail-nav { display: grid; gap: .75rem; }
.rail-item, .rail-locale { width: 2.75rem; height: 2.75rem; display: grid; place-items: center; border: 1px solid transparent; color: var(--muted); background: transparent; }
.rail-item--active, .rail-item:hover, .rail-locale:hover { color: var(--text); border-color: var(--border); background: var(--surface); }
.rail-locale { margin-top: auto; font-size: .75rem; }
.workspace { width: min(100% - 3rem, 90rem); margin: 0 auto; padding: 2.5rem 0 4rem; }
.topbar { display: flex; justify-content: space-between; gap: 1rem; align-items: end; margin-bottom: 2rem; }
.eyebrow { margin: 0 0 .35rem; color: var(--muted); font-size: .72rem; letter-spacing: .14em; text-transform: uppercase; }
h1, h2, p { margin-top: 0; }
h1 { margin-bottom: 0; font-size: clamp(1.6rem, 3vw, 2.6rem); font-weight: 500; letter-spacing: -.03em; }
h2 { margin-bottom: 0; font-size: 1rem; font-weight: 500; }
.site-link { color: var(--muted); text-decoration: none; font-size: .85rem; }
.site-link:hover { color: var(--text); }
.panel { background: var(--surface); border: 1px solid var(--border); padding: 1.25rem; }
.panel-heading { display: flex; justify-content: space-between; gap: 1rem; align-items: start; margin-bottom: 1rem; }
.connection { margin-bottom: 1rem; }
.connection-grid { display: grid; grid-template-columns: 1fr 1fr 1fr; gap: 1rem; }
label { display: grid; gap: .45rem; color: var(--muted); font-size: .78rem; }
label small { color: #697272; }
input, textarea { width: 100%; color: var(--text); background: var(--surface-raised); border: 1px solid var(--border); border-radius: 0; padding: .7rem .75rem; }
input:focus, textarea:focus { border-color: var(--accent); outline: 0; }
.inline-control { display: flex; gap: .5rem; }
.inline-control input { min-width: 0; }
.workspace-grid, .workflow-grid { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(0, .9fr); gap: 1rem; margin-bottom: 1rem; }
.json-editor, .json-preview { min-height: 28rem; font: .78rem/1.65 "SFMono-Regular", Consolas, monospace; tab-size: 2; }
.json-editor { resize: vertical; }
.json-preview { overflow: auto; margin: 0; color: #b6c0bd; background: #0f1314; border: 1px solid var(--border); padding: 1rem; white-space: pre-wrap; word-break: break-word; }
.preview-badge, .status-pill { color: var(--muted); border: 1px solid var(--border); padding: .25rem .45rem; font-size: .68rem; letter-spacing: .08em; text-transform: uppercase; }
.status-pill--succeeded { color: #aec3b5; border-color: #496455; }
.status-pill--failed { color: var(--danger); border-color: #714d47; }
.status-dot { width: .55rem; height: .55rem; border-radius: 50%; background: var(--border); }
.status-dot--active { background: var(--warm); }
.meta-stack { display: grid; gap: .2rem; text-align: right; color: var(--muted); font-size: .72rem; }
.action-row { display: flex; gap: .55rem; margin-top: 1rem; }
.action-row--wrap { flex-wrap: wrap; }
.button { min-height: 2.75rem; border: 1px solid var(--border); background: transparent; color: var(--text); padding: .6rem .85rem; }
.button:hover:not(:disabled) { border-color: var(--accent); background: var(--surface-soft); }
.button--primary { color: #111615; background: var(--accent); border-color: var(--accent); }
.button--primary:hover:not(:disabled) { background: #c0cdca; }
.button--quiet { min-height: 2.5rem; color: var(--muted); white-space: nowrap; }
.button--danger { color: var(--danger); }
.button:disabled { cursor: not-allowed; opacity: .42; }
.reason-field { flex: 1 1 12rem; }
.field-error, .notice { color: var(--warm); font-size: .82rem; }
.notice { border-left: 2px solid var(--warm); padding: .7rem .85rem; background: var(--surface); }
.notice--error { color: var(--danger); border-color: var(--danger); }
.job-meta { margin: 1rem 0 0; color: var(--muted); font: .72rem monospace; overflow-wrap: anywhere; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@media (max-width: 760px) {
  .admin-shell { display: block; }
  .rail { height: 4rem; flex-direction: row; justify-content: space-between; border-right: 0; border-bottom: 1px solid var(--border); padding: .7rem 1rem; }
  .rail-nav { display: flex; gap: .35rem; }
  .rail-locale { margin-top: 0; }
  .workspace { width: min(100% - 1.5rem, 90rem); padding-top: 1.5rem; }
  .topbar { align-items: start; flex-direction: column; }
  .connection-grid, .workspace-grid, .workflow-grid { grid-template-columns: 1fr; }
  .json-editor, .json-preview { min-height: 20rem; }
}
</style>
