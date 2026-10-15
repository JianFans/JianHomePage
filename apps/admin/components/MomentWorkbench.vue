<script setup lang="ts">
import { Images, Plus, X } from '@lucide/vue'
import { computed, ref, shallowRef, watch } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { createMoment, type MomentDraft, type MomentErrorCode } from '../utils/moment-workbench'
import { momentCopy } from '../utils/moment-copy'
import MomentFields from './MomentFields.vue'

const draft = defineModel<MomentDraft | null>('draft', { required: true })
const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  locale: AdminLocale
  busy: boolean
  readOnly?: boolean
  dirty: boolean
  stale: boolean
  error: MomentErrorCode | null
  issues: readonly SnapshotEditorIssue[]
  pending: boolean
  studio?: boolean
}>()
const emit = defineEmits<{ open: []; apply: []; close: []; keep: []; discard: [] }>()
const copy = computed(() => momentCopy(props.locale))
const selectedIndex = ref(0)
const selected = computed(() => draft.value?.moments[selectedIndex.value] ?? null)
const editSnapshot = shallowRef(props.snapshot)

watch(() => props.snapshot, /** JSON 无效时保留最近有效素材选项，应用仍检查最新原文。 */ (snapshot) => {
  if (snapshot) editSnapshot.value = snapshot
})
watch(() => draft.value, /** 重载从首条开始，切换记录只改变位置并保留集合输入。 */ () => { selectedIndex.value = 0 })
watch(() => draft.value?.moments.length, /** 防御外部替换数组后选择越界。 */ (length) => {
  if (selectedIndex.value >= (length ?? 0)) selectedIndex.value = 0
})

/** 新记录仅进入草稿，不自动添加首页引用。 */
function addMoment() {
  if (!draft.value || props.busy || props.readOnly) return
  draft.value.moments.push(createMoment())
  selectedIndex.value = draft.value.moments.length - 1
}
</script>

<template>
  <section
    class="panel music-panel"
    aria-labelledby="moment-title"
    data-testid="moment-workbench"
  >
    <header class="panel-heading">
      <div>
        <p class="eyebrow">
          <Images
            :size="16"
            aria-hidden="true"
          />
        </p><h2 id="moment-title">
          {{ copy.title }}
        </h2>
      </div>
      <button
        type="button"
        class="button"
        data-testid="moment-open"
        :disabled="busy || !snapshot"
        @click="emit('open')"
      >
        {{ draft ? copy.reload : copy.open }}
      </button>
    </header>
    <div
      v-if="pending"
      class="music-notice"
      data-testid="moment-discard-prompt"
      role="alert"
    >
      <p>{{ copy.discardPrompt }}</p>
      <div class="music-tools">
        <button
          type="button"
          class="button"
          data-testid="moment-keep"
          :disabled="busy"
          @click="emit('keep')"
        >
          {{ copy.keep }}
        </button>
        <button
          type="button"
          class="button button--danger"
          data-testid="moment-discard"
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
      data-testid="moment-error"
    >
      {{ copy.errors[error] }}
      <ul v-if="issues.length">
        <li
          v-for="(issue, index) in issues.slice(0, 8)"
          :key="`${issue.path}-${index}`"
        >
          <code>{{ issue.path }}</code> · {{ issue.code }}
        </li>
      </ul>
      <p v-if="issues.length > 8">
        {{ copy.moreIssues(issues.length - 8) }}
      </p>
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
            data-testid="moment-add"
            :aria-label="copy.add"
            :title="copy.add"
            :disabled="busy || readOnly"
            @click="addMoment"
          >
            <Plus
              :size="18"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="icon-tool"
            data-testid="moment-close"
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
            v-for="(moment, index) in draft.moments"
            :key="index"
            type="button"
            class="music-release"
            :class="{ 'music-release--active': selectedIndex === index }"
            :aria-pressed="selectedIndex === index"
            :disabled="busy"
            :data-testid="`moment-record-${moment.id}`"
            data-snapshot-record-kind="moments"
            :data-snapshot-record-index="index"
            @click="selectedIndex = index"
          >
            <Images
              :size="24"
              aria-hidden="true"
            /><span><strong>{{ moment.caption?.[locale] || moment.caption?.['zh-CN'] || moment.id }}</strong></span>
          </button>
        </nav>
        <div
          v-if="selected"
          :key="selected.id"
          class="music-editor"
        >
          <div class="music-editor-heading">
            <code data-testid="moment-id">{{ selected.id }}</code>
          </div>
          <fieldset
            class="music-fields"
            data-testid="moment-fields"
            :disabled="busy || readOnly"
          >
            <MomentFields
              v-if="editSnapshot"
              v-model="selected"
              :snapshot="editSnapshot"
              :draft="draft"
              :locale="locale"
              :path="`/moments/${selectedIndex}`"
              :issues="issues"
            />
          </fieldset>
        </div>
      </div>
      <div class="music-apply-row">
        <button
          type="button"
          class="button button--primary"
          data-testid="moment-apply"
          :disabled="busy || readOnly || stale || !dirty"
          @click="emit('apply')"
        >
          {{ copy.apply }}
        </button>
      </div>
    </template>
  </section>
</template>
