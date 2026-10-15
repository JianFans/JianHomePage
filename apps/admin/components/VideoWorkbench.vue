<script setup lang="ts">
import { Film, Plus, X } from '@lucide/vue'
import { computed, ref, shallowRef, useId, watch } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { createVideo, type VideoDraft, type VideoErrorCode } from '../utils/video-workbench'
import { videoCopy } from '../utils/video-copy'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import LocalizedTextFields from './LocalizedTextFields.vue'
import PlatformLinkFields from './PlatformLinkFields.vue'

const draft = defineModel<VideoDraft | null>('draft', { required: true })
const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  locale: AdminLocale
  busy: boolean
  readOnly?: boolean
  dirty: boolean
  stale: boolean
  error: VideoErrorCode | null
  issues: readonly SnapshotEditorIssue[]
  pending: boolean
  studio?: boolean
}>()
const emit = defineEmits<{ open: []; apply: []; close: []; keep: []; discard: [] }>()
const copy = computed(() => videoCopy(props.locale))
const selectedIndex = ref(0)
const selected = computed(() => draft.value?.videos[selectedIndex.value] ?? null)
const editSnapshot = shallowRef(props.snapshot)
const posters = computed(() => editSnapshot.value?.assets.filter(asset => ['image', 'gif'].includes(asset.kind)) ?? [])
const videoAssets = computed(() => editSnapshot.value?.assets.filter(asset => asset.kind === 'video') ?? [])
const issueId = useId()

/** 记录切换后按当前索引绑定 canonical 字段及诊断说明。 */
function field(name: string) {
  const path = `/videos/${selectedIndex.value}/${name}`
  return issueFieldAttributes(props.issues.find(issue => issue.path === path), path, `${issueId}-${name}`, props.locale)
}

watch(() => props.snapshot, /** JSON 无效时保留最近有效素材选项，应用仍检查最新原文。 */ (snapshot) => {
  if (snapshot) editSnapshot.value = snapshot
})
watch(() => draft.value, /** 重载从首条开始，切换记录只改变位置并保留集合输入。 */ () => { selectedIndex.value = 0 })
watch(() => draft.value?.videos.length, /** 防御外部替换数组后选择越界。 */ (length) => {
  if (selectedIndex.value >= (length ?? 0)) selectedIndex.value = 0
})

/** 新记录仅进入草稿，不自动添加首页引用。 */
function addVideo() {
  if (!draft.value || props.busy || props.readOnly) return
  draft.value.videos.push(createVideo())
  selectedIndex.value = draft.value.videos.length - 1
}
/** 清空选填素材必须移除属性，避免留下空引用。 */
function setVideoAsset(event: Event) {
  if (!selected.value || props.busy || props.readOnly) return
  const id = (event.target as HTMLSelectElement).value
  if (id) selected.value.videoAssetId = id
  else delete selected.value.videoAssetId
}
</script>

<template>
  <section
    class="panel music-panel"
    aria-labelledby="video-title"
    data-testid="video-workbench"
  >
    <header class="panel-heading">
      <div>
        <p class="eyebrow">
          <Film
            :size="16"
            aria-hidden="true"
          />
        </p><h2 id="video-title">
          {{ copy.title }}
        </h2>
      </div>
      <button
        type="button"
        class="button"
        data-testid="video-open"
        :disabled="busy || !snapshot"
        @click="emit('open')"
      >
        {{ draft ? copy.reload : copy.open }}
      </button>
    </header>
    <div
      v-if="pending"
      class="music-notice"
      data-testid="video-discard-prompt"
      role="alert"
    >
      <p>{{ copy.discardPrompt }}</p>
      <div class="music-tools">
        <button
          type="button"
          class="button"
          data-testid="video-keep"
          :disabled="busy"
          @click="emit('keep')"
        >
          {{ copy.keep }}
        </button>
        <button
          type="button"
          class="button button--danger"
          data-testid="video-discard"
          :disabled="busy"
          @click="emit('discard')"
        >
          {{ copy.discard }}
        </button>
      </div>
    </div>
    <p
      v-if="stale"
      class="music-notice"
      role="status"
    >
      {{ copy.stale }}
    </p>
    <div
      v-if="error && !studio"
      class="music-error"
      role="alert"
      data-testid="video-error"
    >
      {{ copy.errors[error] }}
      <ul v-if="issues.length">
        <li
          v-for="(issue, index) in issues"
          :key="`${issue.path}-${index}`"
        >
          <code>{{ issue.path }}</code> · {{ issue.code }}
        </li>
      </ul>
    </div>
    <p
      v-if="!snapshot && !draft"
      class="music-empty"
    >
      {{ copy.empty }}
    </p>
    <template v-if="draft">
      <div class="music-editor-heading">
        <p class="music-muted">
          {{ copy.placement }}
        </p>
        <div class="music-tools">
          <button
            type="button"
            class="icon-tool"
            data-testid="video-add"
            :aria-label="copy.add"
            :title="copy.add"
            :disabled="busy || readOnly"
            @click="addVideo"
          >
            <Plus
              :size="18"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="icon-tool"
            data-testid="video-close"
            :aria-label="copy.close"
            :title="copy.close"
            :disabled="busy"
            @click="emit('close')"
          >
            <X
              :size="18"
              aria-hidden="true"
            />
          </button>
        </div>
      </div>
      <div class="music-layout">
        <nav
          class="music-release-list"
          :aria-label="copy.title"
        >
          <button
            v-for="(video, index) in draft.videos"
            :key="index"
            type="button"
            class="music-release"
            :class="{ 'music-release--active': selectedIndex === index }"
            :aria-pressed="selectedIndex === index"
            :disabled="busy"
            :data-testid="`video-record-${video.id}`"
            data-snapshot-record-kind="videos"
            :data-snapshot-record-index="index"
            @click="selectedIndex = index"
          >
            <Film
              :size="24"
              aria-hidden="true"
            /><span><strong>{{ video.title[locale] || video.title['zh-CN'] || video.id }}</strong></span>
          </button>
        </nav>
        <div
          v-if="selected"
          class="music-editor"
        >
          <div class="music-editor-heading">
            <code data-testid="video-id">{{ selected.id }}</code>
          </div>
          <fieldset
            class="music-fields"
            data-testid="video-fields"
            :disabled="busy || readOnly"
          >
            <LocalizedTextFields
              v-model="selected.title"
              :locale="locale"
              :label="copy.videoTitle"
              test-id="video-title"
              :path="`/videos/${selectedIndex}/title`"
              :issues="issues"
            />
            <div class="music-field-grid">
              <label><span>{{ copy.poster }}</span><select
                v-model="selected.posterAssetId"
                data-testid="video-poster"
                :data-snapshot-path="field('posterAssetId')['data-snapshot-path']"
                :aria-invalid="field('posterAssetId')['aria-invalid']"
                :aria-describedby="field('posterAssetId')['aria-describedby']"
              ><option value="">{{ copy.select }}</option><option
                v-for="asset in posters"
                :key="asset.id"
                :value="asset.id"
              >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select><small
                v-if="field('posterAssetId').description"
                :id="`${issueId}-posterAssetId`"
                class="sr-only"
              >{{ field('posterAssetId').description }}</small></label>
              <label><span>{{ copy.asset }}</span><select
                :value="selected.videoAssetId || ''"
                data-testid="video-asset"
                :data-snapshot-path="field('videoAssetId')['data-snapshot-path']"
                :aria-invalid="field('videoAssetId')['aria-invalid']"
                :aria-describedby="field('videoAssetId')['aria-describedby']"
                @change="setVideoAsset"
              ><option value="">{{ copy.none }}</option><option
                v-for="asset in videoAssets"
                :key="asset.id"
                :value="asset.id"
              >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select><small
                v-if="field('videoAssetId').description"
                :id="`${issueId}-videoAssetId`"
                class="sr-only"
              >{{ field('videoAssetId').description }}</small></label>
              <label><span>{{ copy.duration }}</span><input
                v-model.number="selected.durationSeconds"
                type="number"
                min="1"
                step="1"
                data-testid="video-duration"
                :data-snapshot-path="field('durationSeconds')['data-snapshot-path']"
                :aria-invalid="field('durationSeconds')['aria-invalid']"
                :aria-describedby="field('durationSeconds')['aria-describedby']"
              ><small
                v-if="field('durationSeconds').description"
                :id="`${issueId}-durationSeconds`"
                class="sr-only"
              >{{ field('durationSeconds').description }}</small></label>
              <label class="music-checkbox"><input
                v-model="selected.featured"
                type="checkbox"
                data-testid="video-featured"
              ><span>{{ copy.featured }}</span></label>
            </div>
            <PlatformLinkFields
              v-model="selected.platformLinks"
              :locale="locale"
              :path="`/videos/${selectedIndex}/platformLinks`"
              :issues="issues"
            />
          </fieldset>
        </div>
      </div>
      <div class="music-apply-row">
        <button
          type="button"
          class="button button--primary"
          data-testid="video-apply"
          :disabled="busy || readOnly || stale || !dirty"
          @click="emit('apply')"
        >
          {{ copy.apply }}
        </button>
      </div>
    </template>
  </section>
</template>
