<script setup lang="ts">
import { useRuntimeConfig } from '#imports'
import { Eye, Languages, Monitor, RotateCcw, Smartphone, X } from '@lucide/vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AdminLocale } from '../utils/admin-locale'
import { createLocaleMessage, createSnapshotMessage, readPreviewMessage } from '../utils/homepage-preview'

const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  textIsCurrent: boolean
  textRevision: number
  formDirty: boolean
  locale: AdminLocale
  autoOpen?: boolean
}>()

const runtime = useRuntimeConfig()
const previewSrc = `${runtime.app.baseURL.replace(/\/$/, '')}/preview`
const opened = ref(false)
const ready = ref(false)
const failed = ref(false)
const generation = ref(0)
const previewLocale = ref<AdminLocale>(props.locale)
const mobile = ref(false)
const frame = ref<HTMLIFrameElement | null>(null)
const stage = ref<HTMLElement | null>(null)
const containerWidth = ref(0)
let timer: ReturnType<typeof setTimeout> | undefined
let observer: ResizeObserver | undefined
let listening = false

const viewport = computed(() => mobile.value ? { width: 390, height: 780 } : { width: 1280, height: 800 })
const scale = computed(() => Math.min(1, containerWidth.value / viewport.value.width))
const canRender = computed(() => opened.value && props.textIsCurrent && Boolean(props.snapshot) && !failed.value)
const labels = computed(() => props.locale === 'en'
  ? {
      title: 'Homepage preview',
      open: 'Open homepage preview',
      close: 'Close homepage preview',
      desktop: 'Desktop viewport',
      mobile: 'Mobile viewport',
      language: 'Switch preview language',
      retry: 'Retry preview',
      waiting: 'Waiting for homepage preview…',
      invalid: 'Preview resumes when the current JSON is valid.',
      error: 'Preview could not load.',
      unapplied: 'Showing applied content; form changes have not been applied.',
    }
  : {
      title: '首页预览',
      open: '打开首页预览',
      close: '关闭首页预览',
      desktop: '桌面视口',
      mobile: '手机视口',
      language: '切换预览语言',
      retry: '重试预览',
      waiting: '正在等待首页预览…',
      invalid: '当前 JSON 有效后恢复预览。',
      error: '预览加载失败。',
      unapplied: '当前显示已应用内容，表单修改尚未应用。',
    })

/** Clear every resource owned by a document session before replacing or removing its iframe. */
function stopSession() {
  if (timer) clearTimeout(timer)
  timer = undefined
  if (listening) window.removeEventListener('message', receive)
  listening = false
  observer?.disconnect()
  observer = undefined
  ready.value = false
}

/** Install the listener before Vue mounts the new document; a ready-before-load handshake is valid. */
function startSession() {
  stopSession()
  frame.value = null
  generation.value++
  failed.value = false
  if (!opened.value || !props.textIsCurrent || !props.snapshot) return
  window.addEventListener('message', receive)
  listening = true
  timer = setTimeout(() => {
    stopSession()
    failed.value = true
  }, 10000)
}

/** Accept events only from the current iframe, and transmit an explicit content-only object. */
function receive(event: MessageEvent) {
  const source = frame.value?.contentWindow
  if (!source || !canRender.value) return
  const message = readPreviewMessage(event, location.origin, source)
  if (!message) return
  if (message.type === 'ready' && props.snapshot) {
    if (timer) clearTimeout(timer)
    timer = undefined
    ready.value = true
    source.postMessage(createSnapshotMessage({ snapshot: props.snapshot, locale: previewLocale.value }), location.origin)
  } else if (message.type === 'locale' && ready.value) {
    previewLocale.value = message.locale
  }
}

/** Opening creates a document session; no content or credentials are persisted. */
function open() {
  previewLocale.value = props.locale
  opened.value = true
  startSession()
}

/** Closing truly unmounts the document and ends media ownership inside it. */
function close() {
  opened.value = false
  stopSession()
}

/** Keep the preview language independent of the editor language for this open session. */
function toggleLocale() {
  previewLocale.value = previewLocale.value === 'zh-CN' ? 'en' : 'zh-CN'
  if (ready.value) frame.value?.contentWindow?.postMessage(createLocaleMessage(previewLocale.value), location.origin)
}

/** Scale the complete document while retaining its actual desktop or phone layout viewport. */
function observeStage(element: HTMLElement | null) {
  observer?.disconnect()
  observer = undefined
  if (!element) return
  containerWidth.value = element.getBoundingClientRect().width
  observer = new ResizeObserver((entries) => {
    const entry = entries[0]
    if (entry) containerWidth.value = entry.contentRect.width
  })
  observer.observe(element)
}

// The revision also invalidates an edit-and-undo within one Vue tick, when equality alone misses it.
watch([() => props.textIsCurrent, () => props.snapshot, () => props.textRevision], startSession, { flush: 'sync' })
watch([stage, generation], () => observeStage(stage.value), { flush: 'post' })
onBeforeUnmount(stopSession)
onMounted(() => { if (props.autoOpen) open() })
</script>

<template>
  <section
    class="homepage-preview panel"
    :aria-label="labels.title"
    data-testid="homepage-preview"
  >
    <div class="homepage-preview__toolbar">
      <h2>{{ labels.title }}</h2>
      <div class="homepage-preview__tools">
        <button
          v-if="!opened"
          type="button"
          :aria-label="labels.open"
          :title="labels.open"
          data-testid="preview-open"
          @click="open"
        >
          <Eye aria-hidden="true" />
        </button>
        <template v-else>
          <button
            type="button"
            :aria-label="labels.desktop"
            :title="labels.desktop"
            :aria-pressed="!mobile"
            data-testid="preview-desktop"
            @click="mobile = false"
          >
            <Monitor aria-hidden="true" />
          </button>
          <button
            type="button"
            :aria-label="labels.mobile"
            :title="labels.mobile"
            :aria-pressed="mobile"
            data-testid="preview-mobile"
            @click="mobile = true"
          >
            <Smartphone aria-hidden="true" />
          </button>
          <button
            type="button"
            :aria-label="labels.language"
            :title="labels.language"
            data-testid="preview-language"
            @click="toggleLocale"
          >
            <Languages aria-hidden="true" /><span>{{ previewLocale === 'en' ? 'EN' : '中' }}</span>
          </button>
          <button
            type="button"
            :aria-label="labels.close"
            :title="labels.close"
            data-testid="preview-close"
            @click="close"
          >
            <X aria-hidden="true" />
          </button>
        </template>
      </div>
    </div>
    <template v-if="opened">
      <p
        v-if="formDirty"
        role="status"
        data-testid="preview-form-dirty"
      >
        {{ labels.unapplied }}
      </p>
      <p
        v-if="!textIsCurrent || !snapshot"
        role="status"
        data-testid="preview-invalid"
      >
        {{ labels.invalid }}
      </p>
      <div
        v-else-if="failed"
        role="status"
        data-testid="preview-error"
        class="homepage-preview__error"
      >
        <p>{{ labels.error }}</p>
        <button
          type="button"
          :aria-label="labels.retry"
          :title="labels.retry"
          data-testid="preview-retry"
          @click="startSession"
        >
          <RotateCcw aria-hidden="true" />
        </button>
      </div>
      <p
        v-else-if="!ready"
        role="status"
        data-testid="preview-waiting"
      >
        {{ labels.waiting }}
      </p>
      <div
        v-if="canRender"
        ref="stage"
        class="homepage-preview__stage"
      >
        <div
          class="homepage-preview__viewport"
          :style="{ width: `${viewport.width * scale}px`, height: `${viewport.height * scale}px` }"
        >
          <iframe
            :key="generation"
            ref="frame"
            :src="previewSrc"
            :title="labels.title"
            :width="viewport.width"
            :height="viewport.height"
            :style="{ width: `${viewport.width}px`, height: `${viewport.height}px`, transform: `scale(${scale})` }"
            sandbox="allow-scripts allow-same-origin"
            referrerpolicy="no-referrer"
            data-testid="preview-frame"
          />
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.homepage-preview { min-width: 0; margin-top: 1.25rem; }
.homepage-preview__toolbar { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: .75rem; }
.homepage-preview__toolbar h2 { margin: 0; font-size: 1rem; }
.homepage-preview__tools { display: flex; flex-wrap: wrap; gap: .25rem; }
.homepage-preview button { display: inline-flex; min-width: 44px; min-height: 44px; align-items: center; justify-content: center; gap: .2rem; padding: .4rem; border: 1px solid #333d3f; border-radius: 4px; background: #171d1f; color: #c5cecd; cursor: pointer; }
.homepage-preview button[aria-pressed="true"] { border-color: #8caaa5; }
.homepage-preview button:focus-visible { outline: 2px solid #c4d7d3; outline-offset: 2px; }
.homepage-preview button svg { width: 18px; height: 18px; }
.homepage-preview p { color: #9daaa8; font-size: .85rem; }
.homepage-preview__stage { width: 100%; min-width: 0; overflow: hidden; margin-top: .75rem; background: #0d0f11; }
.homepage-preview__viewport { position: relative; max-width: 100%; margin-inline: auto; overflow: hidden; }
.homepage-preview iframe { position: absolute; top: 0; left: 0; display: block; max-width: none; border: 0; transform-origin: top left; }
.homepage-preview__error { display: flex; align-items: center; gap: 1rem; }
</style>
