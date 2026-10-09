<script setup lang="ts">
import {
  AlertCircle,
  Check,
  ChevronDown,
  FileAudio,
  FileImage,
  Film,
  LoaderCircle,
  Plus,
  RefreshCw,
  RotateCcw,
  Search,
  Upload,
  Video,
} from '@lucide/vue'
import { computed, reactive, ref, toRef, watch } from 'vue'
import { useAssetWorkspace } from '../composables/useAssetWorkspace'
import type { AdminAsset } from '../utils/admin-api'
import { insertSnapshotAsset, toSnapshotAsset, type LocalizedDraft } from '../utils/asset-workbench'

const props = defineProps<{
  locale: 'zh-CN' | 'en'
  editorText: string
  apiBaseUrl: string
  token: string
}>()

const emit = defineEmits<{
  'update:editorText': [value: string]
}>()

const workspace = reactive(useAssetWorkspace({
  apiBaseUrl: toRef(props, 'apiBaseUrl'),
  token: toRef(props, 'token'),
}))
const fileInput = ref<HTMLInputElement | null>(null)
const assetAlts = reactive<Record<string, LocalizedDraft>>({})
const insertError = ref('')
const pendingAlt = ref<LocalizedDraft | null>(null)

const copy = computed(() => props.locale === 'en'
  ? {
      title: 'Assets',
      upload: 'Upload',
      library: 'Library',
      choose: 'Choose file',
      sourceZh: 'Source · Chinese',
      sourceEn: 'Source · English',
      credit: 'Credit',
      license: 'License',
      altZh: 'Alt text · Chinese',
      altEn: 'Alt text · English',
      uploadAction: 'Upload asset',
      retryComplete: 'Retry confirmation',
      refresh: 'Refresh assets',
      search: 'Search assets',
      statusFilter: 'Status',
      kindFilter: 'Type',
      allStatuses: 'All active',
      pending: 'Pending',
      ready: 'Ready',
      deleted: 'Deleted',
      allKinds: 'All types',
      image: 'Image',
      gif: 'GIF',
      audio: 'Audio',
      video: 'Video',
      insert: 'Insert into snapshot',
      duplicate: 'Already in snapshot',
      loadMore: 'Load more',
      empty: 'No matching assets',
      connection: 'Add an API token, then refresh the library.',
      stages: {
        idle: 'Ready', hashing: 'Hashing file', creating: 'Creating upload', uploading: 'Uploading file',
        completing: 'Confirming upload', succeeded: 'Upload complete', failed: 'Upload failed',
      },
      errors: {
        'file-required': 'Choose a file first',
        'form-invalid': 'Chinese source and alt text are required',
        'empty-file': 'File cannot be empty',
        'unsupported-type': 'File type is not supported',
        'invalid-extension': 'File extension does not match its type',
        'file-too-large': 'File exceeds the allowed size',
        'hashing-failed': 'Unable to hash the file',
        'create-failed': 'Unable to create an upload',
        'upload-failed': 'Direct upload failed; retry to request a new signature',
        'complete-failed': 'Upload confirmation failed',
        'list-failed': 'Unable to load assets',
      },
      insertErrors: {
        'invalid-snapshot': 'Fix the snapshot before inserting an asset',
        'duplicate-id': 'This asset already exists in the snapshot',
        'invalid-asset': 'The asset does not satisfy the snapshot contract',
        conversion: 'The server asset metadata is incomplete',
      },
    }
  : {
      title: '素材',
      upload: '上传',
      library: '素材库',
      choose: '选择文件',
      sourceZh: '来源 · 中文',
      sourceEn: '来源 · 英文',
      credit: '署名',
      license: '许可',
      altZh: '替代文本 · 中文',
      altEn: '替代文本 · 英文',
      uploadAction: '上传素材',
      retryComplete: '重试确认',
      refresh: '刷新素材',
      search: '搜索素材',
      statusFilter: '状态',
      kindFilter: '类型',
      allStatuses: '未删除',
      pending: '待上传',
      ready: '可用',
      deleted: '已删除',
      allKinds: '全部类型',
      image: '图片',
      gif: '动图',
      audio: '音频',
      video: '视频',
      insert: '插入快照',
      duplicate: '已在快照中',
      loadMore: '加载更多',
      empty: '没有匹配素材',
      connection: '填写 API Token 后刷新素材库。',
      stages: {
        idle: '就绪', hashing: '正在计算摘要', creating: '正在创建上传', uploading: '正在上传文件',
        completing: '正在确认上传', succeeded: '上传完成', failed: '上传失败',
      },
      errors: {
        'file-required': '请先选择文件',
        'form-invalid': '中文来源和替代文本不能为空',
        'empty-file': '文件不能为空',
        'unsupported-type': '不支持该文件类型',
        'invalid-extension': '扩展名与文件类型不一致',
        'file-too-large': '文件超过允许大小',
        'hashing-failed': '无法计算文件摘要',
        'create-failed': '无法创建上传',
        'upload-failed': '直传失败，请重试以获取新签名',
        'complete-failed': '上传确认失败',
        'list-failed': '无法加载素材',
      },
      insertErrors: {
        'invalid-snapshot': '请先修复当前快照',
        'duplicate-id': '素材已存在于快照中',
        'invalid-asset': '素材不符合快照契约',
        conversion: '服务端素材信息不完整',
      },
    })

const activeUpload = computed(() => ['hashing', 'creating', 'uploading', 'completing'].includes(workspace.stage))
const existingAssetIds = computed(() => {
  try {
    const value = JSON.parse(props.editorText) as { assets?: unknown }
    return new Set(Array.isArray(value.assets)
      ? value.assets.flatMap(item => item && typeof item === 'object' && 'id' in item ? [String(item.id)] : [])
      : [])
  } catch {
    return new Set<string>()
  }
})

watch(() => workspace.assets, (items) => {
  items.forEach((asset) => {
    assetAlts[asset.id] ||= { zhCN: '', en: '' }
  })
}, { deep: false, immediate: true })

/** 打开原生文件选择器，同时保留键盘和辅助技术入口。 */
function chooseFile() {
  fileInput.value?.click()
}

/** 保存用户选择的单个文件并允许再次选择同名文件。 */
function handleFile(event: Event) {
  const input = event.currentTarget as HTMLInputElement
  workspace.file = input.files?.[0] || null
  input.value = ''
}

/** 执行上传，并把上传表单中的替代文本带到新素材卡片。 */
async function uploadAsset() {
  pendingAlt.value = { zhCN: workspace.altZhCN, en: workspace.altEn }
  await workspace.upload()
  restoreCompletedAlt()
}

/** 重试完成确认后恢复首次上传时填写的替代文本。 */
async function retryComplete() {
  await workspace.retryComplete()
  restoreCompletedAlt()
}

/** 将待确认上传的替代文本绑定到新完成的素材卡片。 */
function restoreCompletedAlt() {
  const completed = workspace.assets[0]
  if (workspace.stage !== 'succeeded' || !completed || !pendingAlt.value) return
  assetAlts[completed.id] = pendingAlt.value
  pendingAlt.value = null
}

/** 将素材转换并追加到当前编辑器文本，不触发自动保存。 */
function insertAsset(asset: AdminAsset) {
  insertError.value = ''
  try {
    const result = insertSnapshotAsset(props.editorText, toSnapshotAsset(asset, assetAlts[asset.id] || { zhCN: '' }))
    if (!result.inserted) {
      insertError.value = copy.value.insertErrors[result.error || 'invalid-asset']
      return
    }
    emit('update:editorText', result.text)
  } catch {
    insertError.value = copy.value.insertErrors.conversion
  }
}

/** 仅允许可用、未重复且具有中文替代文本的素材进入快照。 */
function canInsert(asset: AdminAsset): boolean {
  return asset.status === 'ready'
    && !existingAssetIds.value.has(asset.id)
    && Boolean(assetAlts[asset.id]?.zhCN.trim())
}

/** 相对媒体地址按 API Origin 预览，保留素材本身的 canonical 地址。 */
function previewSource(asset: AdminAsset): string {
  if (!asset.src.startsWith('/media/')) return asset.src
  try {
    return new URL(asset.src, props.apiBaseUrl).href
  } catch {
    return ''
  }
}

/** 判断素材是否可以使用固定比例安全预览。 */
function isVisual(asset: AdminAsset): boolean {
  return asset.status === 'ready' && (asset.metadata.contentType === 'image/webp' || asset.metadata.contentType === 'image/gif')
}

/** 为非视觉素材或未就绪素材选择稳定图标。 */
function assetIcon(asset: AdminAsset) {
  if (asset.metadata.contentType === 'video/mp4') return Video
  if (asset.metadata.contentType === 'audio/wav' || asset.metadata.contentType === 'audio/mpeg') return FileAudio
  if (asset.metadata.contentType === 'image/gif') return Film
  return FileImage
}

/** 以紧凑单位显示服务端声明的文件大小。 */
function formatBytes(value: unknown): string {
  if (typeof value !== 'number' || !Number.isFinite(value) || value <= 0) return '—'
  if (value >= 1024 * 1024 * 1024) return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GiB`
  if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MiB`
  if (value >= 1024) return `${(value / 1024).toFixed(1)} KiB`
  return `${value} B`
}
</script>

<template>
  <section
    class="asset-workbench panel"
    aria-labelledby="asset-workbench-title"
    data-testid="asset-workbench"
  >
    <div class="panel-heading asset-heading">
      <div>
        <p class="eyebrow">
          04
        </p>
        <h2 id="asset-workbench-title">
          {{ copy.title }}
        </h2>
      </div>
      <div
        class="asset-stage"
        :class="`asset-stage--${workspace.stage}`"
        aria-live="polite"
        data-testid="asset-stage"
      >
        <LoaderCircle
          v-if="activeUpload"
          class="asset-spinner"
          :size="15"
          aria-hidden="true"
        />
        <Check
          v-else-if="workspace.stage === 'succeeded'"
          :size="15"
          aria-hidden="true"
        />
        <AlertCircle
          v-else-if="workspace.stage === 'failed'"
          :size="15"
          aria-hidden="true"
        />
        <span>{{ copy.stages[workspace.stage] }}</span>
      </div>
    </div>

    <div class="asset-layout">
      <form
        class="asset-upload"
        @submit.prevent="uploadAsset"
      >
        <div class="asset-subheading">
          <Upload
            :size="16"
            aria-hidden="true"
          />
          <span>{{ copy.upload }}</span>
        </div>

        <button
          class="file-picker"
          type="button"
          :disabled="activeUpload"
          @click="chooseFile"
        >
          <Plus
            :size="18"
            aria-hidden="true"
          />
          <span>{{ workspace.file?.name || copy.choose }}</span>
          <small v-if="workspace.file">{{ formatBytes(workspace.file.size) }}</small>
        </button>
        <input
          ref="fileInput"
          class="sr-only"
          type="file"
          accept="image/webp,.webp,image/gif,.gif,audio/mpeg,.mp3,audio/wav,.wav,video/mp4,.mp4"
          :aria-label="copy.choose"
          data-testid="asset-file-input"
          @change="handleFile"
        >

        <div class="asset-form-grid">
          <label>
            <span>{{ copy.sourceZh }}</span>
            <input
              v-model="workspace.sourceZhCN"
              required
              type="text"
              :aria-label="copy.sourceZh"
              data-testid="asset-source-zh"
            >
          </label>
          <label>
            <span>{{ copy.sourceEn }}</span>
            <input
              v-model="workspace.sourceEn"
              type="text"
              :aria-label="copy.sourceEn"
            >
          </label>
          <label>
            <span>{{ copy.credit }}</span>
            <input
              v-model="workspace.credit"
              type="text"
              :aria-label="copy.credit"
            >
          </label>
          <label>
            <span>{{ copy.license }}</span>
            <input
              v-model="workspace.license"
              type="text"
              :aria-label="copy.license"
            >
          </label>
          <label>
            <span>{{ copy.altZh }}</span>
            <input
              v-model="workspace.altZhCN"
              required
              type="text"
              :aria-label="copy.altZh"
              data-testid="asset-alt-upload-zh"
            >
          </label>
          <label>
            <span>{{ copy.altEn }}</span>
            <input
              v-model="workspace.altEn"
              type="text"
              :aria-label="copy.altEn"
            >
          </label>
        </div>

        <div class="asset-actions">
          <button
            class="button button--primary"
            type="submit"
            :disabled="activeUpload || !workspace.file"
            data-testid="asset-upload"
          >
            <Upload
              :size="16"
              aria-hidden="true"
            />
            <span>{{ copy.uploadAction }}</span>
          </button>
          <button
            v-if="workspace.canRetryComplete"
            class="button"
            type="button"
            @click="retryComplete"
          >
            <RotateCcw
              :size="16"
              aria-hidden="true"
            />
            <span>{{ copy.retryComplete }}</span>
          </button>
        </div>
        <p
          v-if="workspace.errorCode"
          class="asset-error"
          role="alert"
        >
          {{ copy.errors[workspace.errorCode] }}
        </p>
      </form>

      <div class="asset-library">
        <div class="asset-library-bar">
          <div class="asset-subheading">
            <FileImage
              :size="16"
              aria-hidden="true"
            />
            <span>{{ copy.library }}</span>
          </div>
          <button
            class="icon-tool"
            type="button"
            :aria-label="copy.refresh"
            :title="copy.refresh"
            :disabled="workspace.loading || !token.trim()"
            data-testid="asset-refresh"
            @click="workspace.loadAssets()"
          >
            <RefreshCw
              :size="16"
              aria-hidden="true"
            />
          </button>
        </div>

        <div class="asset-filters">
          <label class="search-control">
            <span class="sr-only">{{ copy.search }}</span>
            <Search
              :size="15"
              aria-hidden="true"
            />
            <input
              v-model="workspace.searchText"
              type="search"
              :placeholder="copy.search"
              :aria-label="copy.search"
            >
          </label>
          <label class="select-control">
            <span class="sr-only">{{ copy.statusFilter }}</span>
            <select
              :value="workspace.statusFilter"
              :aria-label="copy.statusFilter"
              data-testid="asset-status-filter"
              @change="workspace.setStatusFilter(($event.target as HTMLSelectElement).value as AdminAsset['status'] | '')"
            >
              <option value="">{{ copy.allStatuses }}</option>
              <option value="pending">{{ copy.pending }}</option>
              <option value="ready">{{ copy.ready }}</option>
              <option value="deleted">{{ copy.deleted }}</option>
            </select>
            <ChevronDown
              :size="14"
              aria-hidden="true"
            />
          </label>
          <label class="select-control">
            <span class="sr-only">{{ copy.kindFilter }}</span>
            <select
              v-model="workspace.kindFilter"
              :aria-label="copy.kindFilter"
              data-testid="asset-kind-filter"
            >
              <option value="all">{{ copy.allKinds }}</option>
              <option value="image">{{ copy.image }}</option>
              <option value="gif">{{ copy.gif }}</option>
              <option value="audio">{{ copy.audio }}</option>
              <option value="video">{{ copy.video }}</option>
            </select>
            <ChevronDown
              :size="14"
              aria-hidden="true"
            />
          </label>
        </div>

        <p
          v-if="!token.trim()"
          class="asset-empty"
        >
          {{ copy.connection }}
        </p>
        <p
          v-else-if="workspace.listError"
          class="asset-error"
          role="alert"
          data-testid="asset-list-error"
        >
          {{ copy.errors['list-failed'] }}
        </p>
        <p
          v-else-if="!workspace.loading && workspace.filteredAssets.length === 0"
          class="asset-empty"
        >
          {{ copy.empty }}
        </p>

        <div class="asset-grid">
          <article
            v-for="asset in workspace.filteredAssets"
            :key="asset.id"
            class="asset-card"
            :data-asset-id="asset.id"
            data-testid="asset-card"
          >
            <div class="asset-preview">
              <img
                v-if="isVisual(asset)"
                :src="previewSource(asset)"
                :alt="assetAlts[asset.id]?.zhCN || ''"
                loading="lazy"
                decoding="async"
              >
              <component
                :is="assetIcon(asset)"
                v-else
                :size="28"
                aria-hidden="true"
              />
              <span class="asset-status">{{ copy[asset.status] }}</span>
            </div>
            <div class="asset-card-body">
              <div class="asset-meta">
                <strong>{{ asset.metadata.fileName || asset.id }}</strong>
                <small>{{ asset.metadata.contentType || '—' }} · {{ formatBytes(asset.metadata.declaredSize) }}</small>
              </div>
              <label>
                <span>{{ copy.altZh }}</span>
                <input
                  v-model="assetAlts[asset.id]!.zhCN"
                  type="text"
                  :aria-label="`${copy.altZh}: ${asset.id}`"
                  data-testid="asset-alt-zh"
                >
              </label>
              <label>
                <span>{{ copy.altEn }}</span>
                <input
                  v-model="assetAlts[asset.id]!.en"
                  type="text"
                  :aria-label="`${copy.altEn}: ${asset.id}`"
                >
              </label>
              <button
                class="button asset-insert"
                type="button"
                :disabled="!canInsert(asset)"
                :title="existingAssetIds.has(asset.id) ? copy.duplicate : copy.insert"
                data-testid="asset-insert"
                @click="insertAsset(asset)"
              >
                <Plus
                  :size="16"
                  aria-hidden="true"
                />
                <span>{{ existingAssetIds.has(asset.id) ? copy.duplicate : copy.insert }}</span>
              </button>
            </div>
          </article>
        </div>

        <button
          v-if="workspace.nextCursor"
          class="button asset-load-more"
          type="button"
          :disabled="workspace.loading"
          data-testid="asset-load-more"
          @click="workspace.loadMore"
        >
          {{ copy.loadMore }}
        </button>
        <p
          v-if="insertError"
          class="asset-error"
          role="alert"
        >
          {{ insertError }}
        </p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.asset-workbench { margin-bottom: 1rem; }
.asset-heading { align-items: center; }
.asset-layout { display: grid; grid-template-columns: minmax(18rem, .72fr) minmax(0, 1.28fr); gap: 1.25rem; }
.asset-upload { min-width: 0; padding-right: 1.25rem; border-right: 1px solid var(--border); }
.asset-library { min-width: 0; }
.asset-subheading { display: flex; align-items: center; gap: .5rem; min-height: 2.75rem; color: var(--muted); font-size: .72rem; letter-spacing: .08em; text-transform: uppercase; }
.asset-stage { display: inline-flex; align-items: center; gap: .4rem; min-height: 2rem; color: var(--muted); font-size: .72rem; }
.asset-stage--succeeded { color: #aec3b5; }
.asset-stage--failed { color: var(--danger); }
.asset-spinner { animation: asset-spin .9s linear infinite; }
.file-picker { width: 100%; min-height: 4.5rem; display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: .75rem; border: 1px dashed var(--border); background: var(--surface-raised); color: var(--text); padding: .9rem; text-align: left; }
.file-picker:hover:not(:disabled) { border-color: var(--accent); background: var(--surface-soft); }
.file-picker span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-picker small { color: var(--muted); }
.asset-form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .8rem; margin-top: .9rem; }
.asset-form-grid label, .asset-card label { display: grid; gap: .35rem; color: var(--muted); font-size: .7rem; }
.asset-form-grid input, .asset-card input, .asset-filters input, .asset-filters select { width: 100%; color: var(--text); background: var(--surface-raised); border: 1px solid var(--border); border-radius: 0; padding: .65rem .7rem; }
.asset-form-grid input:focus, .asset-card input:focus, .asset-filters input:focus, .asset-filters select:focus { border-color: var(--accent); outline: 0; }
.asset-actions { display: flex; flex-wrap: wrap; gap: .55rem; margin-top: 1rem; }
.asset-actions .button, .asset-insert { display: inline-flex; align-items: center; justify-content: center; gap: .45rem; }
.asset-error { margin: .75rem 0 0; color: var(--danger); font-size: .76rem; }
.asset-library-bar { display: flex; align-items: center; justify-content: space-between; gap: 1rem; }
.asset-filters { display: grid; grid-template-columns: minmax(10rem, 1fr) auto auto; gap: .5rem; margin-bottom: .8rem; }
.search-control, .select-control { position: relative; display: flex; align-items: center; color: var(--muted); }
.search-control > svg { position: absolute; left: .7rem; pointer-events: none; }
.search-control input { padding-left: 2rem; }
.select-control select { min-height: 2.75rem; appearance: none; padding-right: 2rem; }
.select-control > svg { position: absolute; right: .6rem; pointer-events: none; }
.asset-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: .75rem; }
.asset-card { min-width: 0; border: 1px solid var(--border); background: var(--surface-raised); }
.asset-preview { position: relative; aspect-ratio: 16 / 9; display: grid; place-items: center; overflow: hidden; color: var(--muted); background: #0f1314; border-bottom: 1px solid var(--border); }
.asset-preview img { width: 100%; height: 100%; object-fit: cover; }
.asset-status { position: absolute; top: .5rem; right: .5rem; padding: .18rem .38rem; border: 1px solid var(--border); background: rgb(11 13 14 / 88%); color: var(--muted); font-size: .62rem; }
.asset-card-body { display: grid; gap: .65rem; padding: .75rem; }
.asset-meta { min-width: 0; display: grid; gap: .18rem; }
.asset-meta strong { overflow: hidden; color: var(--text); font-size: .78rem; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; }
.asset-meta small { color: var(--muted); font-size: .66rem; overflow-wrap: anywhere; }
.asset-insert { width: 100%; }
.asset-load-more { width: 100%; margin-top: .75rem; }
.asset-empty { min-height: 6rem; display: grid; place-items: center; margin: 0; color: var(--muted); font-size: .76rem; text-align: center; }
.sr-only { position: absolute; width: 1px; height: 1px; padding: 0; margin: -1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; border: 0; }
@keyframes asset-spin { to { transform: rotate(360deg); } }
@media (max-width: 860px) {
  .asset-layout { grid-template-columns: 1fr; }
  .asset-upload { padding-right: 0; padding-bottom: 1.25rem; border-right: 0; border-bottom: 1px solid var(--border); }
}
@media (max-width: 560px) {
  .asset-form-grid, .asset-grid, .asset-filters { grid-template-columns: 1fr; }
  .asset-filters .select-control { width: 100%; }
}
@media (prefers-reduced-motion: reduce) {
  .asset-spinner { animation: none; }
}
</style>
