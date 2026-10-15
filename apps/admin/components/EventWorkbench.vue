<script setup lang="ts">
import { CalendarDays, Plus, X } from '@lucide/vue'
import { computed, ref, shallowRef, useId, watch } from 'vue'
import type { YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from '../utils/admin-locale'
import type { SnapshotEditorIssue } from '../utils/snapshot-workbench'
import { createEvent, type EventDraft, type EventErrorCode } from '../utils/event-workbench'
import { eventCopy } from '../utils/event-copy'
import { issueFieldAttributes } from '../utils/issue-accessibility'
import LocalizedTextFields from './LocalizedTextFields.vue'

const draft = defineModel<EventDraft | null>('draft', { required: true })
const props = defineProps<{
  snapshot: YujianContentSnapshot | null
  locale: AdminLocale
  busy: boolean
  readOnly?: boolean
  dirty: boolean
  stale: boolean
  error: EventErrorCode | null
  issues: readonly SnapshotEditorIssue[]
  pending: boolean
  studio?: boolean
}>()
const emit = defineEmits<{ open: []; apply: []; close: []; keep: []; discard: [] }>()
const copy = computed(() => eventCopy(props.locale))
const selectedIndex = ref(0)
const selected = computed(() => draft.value?.events[selectedIndex.value] ?? null)
const issueId = useId()
const dateTimePath = computed(() => `/events/${selectedIndex.value}/dateTime`)
const dateTimeIssue = computed(() => props.issues.find(issue => issue.path === dateTimePath.value))
const dateTimeIssueAttrs = computed(() => issueFieldAttributes(dateTimeIssue.value, dateTimePath.value, issueId, props.locale))
const detailUrlPath = computed(() => `/events/${selectedIndex.value}/detailUrl`)
const detailUrlIssue = computed(() => props.issues.find(issue => issue.path === detailUrlPath.value))
const detailUrlIssueAttrs = computed(() => issueFieldAttributes(detailUrlIssue.value, detailUrlPath.value, `${issueId}-detail-url`, props.locale))
const editSnapshot = shallowRef(props.snapshot)
const posters = computed(() => editSnapshot.value?.assets.filter(asset => ['image', 'gif'].includes(asset.kind)) ?? [])

watch(() => props.snapshot, /** JSON 无效时保留最近有效素材选项，应用仍检查最新原文。 */ (snapshot) => {
  if (snapshot) editSnapshot.value = snapshot
})
watch(() => draft.value, /** 重载从首条开始，切换记录只改变位置并保留集合输入。 */ () => { selectedIndex.value = 0 })
watch(() => draft.value?.events.length, /** 防御外部替换数组后选择越界。 */ (length) => {
  if (selectedIndex.value >= (length ?? 0)) selectedIndex.value = 0
})

/** 新记录仅进入草稿，不自动添加首页引用。 */
function addEvent() {
  if (!draft.value || props.busy || props.readOnly) return
  draft.value.events.push(createEvent())
  selectedIndex.value = draft.value.events.length - 1
}
/** 清空选填素材必须移除属性，避免留下空引用。 */
function setPosterAsset(event: Event) {
  if (!selected.value || props.busy || props.readOnly) return
  const id = (event.target as HTMLSelectElement).value
  if (id) selected.value.posterAssetId = id
  else delete selected.value.posterAssetId
}
</script>

<template>
  <section
    class="panel music-panel"
    aria-labelledby="event-title"
    data-testid="event-workbench"
  >
    <header class="panel-heading">
      <div>
        <p class="eyebrow">
          <CalendarDays
            :size="16"
            aria-hidden="true"
          />
        </p><h2 id="event-title">
          {{ copy.title }}
        </h2>
      </div>
      <button
        type="button"
        class="button"
        data-testid="event-open"
        :disabled="busy || !snapshot"
        @click="emit('open')"
      >
        {{ draft ? copy.reload : copy.open }}
      </button>
    </header>
    <div
      v-if="pending"
      class="music-notice"
      data-testid="event-discard-prompt"
      role="alert"
    >
      <p>{{ copy.discardPrompt }}</p>
      <div class="music-tools">
        <button
          type="button"
          class="button"
          data-testid="event-keep"
          :disabled="busy"
          @click="emit('keep')"
        >
          {{ copy.keep }}
        </button>
        <button
          type="button"
          class="button button--danger"
          data-testid="event-discard"
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
      data-testid="event-error"
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
            data-testid="event-add"
            :aria-label="copy.add"
            :title="copy.add"
            :disabled="busy || readOnly"
            @click="addEvent"
          >
            <Plus
              :size="18"
              aria-hidden="true"
            />
          </button>
          <button
            type="button"
            class="icon-tool"
            data-testid="event-close"
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
            v-for="(event, index) in draft.events"
            :key="index"
            type="button"
            class="music-release"
            :class="{ 'music-release--active': selectedIndex === index }"
            :aria-pressed="selectedIndex === index"
            :disabled="busy"
            :data-testid="`event-record-${event.id}`"
            data-snapshot-record-kind="events"
            :data-snapshot-record-index="index"
            @click="selectedIndex = index"
          >
            <CalendarDays
              :size="24"
              aria-hidden="true"
            /><span><strong>{{ event.title[locale] || event.title['zh-CN'] || event.id }}</strong></span>
          </button>
        </nav>
        <div
          v-if="selected"
          :key="selected.id"
          class="music-editor"
          data-snapshot-record-kind="events"
          :data-snapshot-record-index="selectedIndex"
        >
          <div class="music-editor-heading">
            <code data-testid="event-id">{{ selected.id }}</code>
          </div>
          <fieldset
            class="music-fields"
            data-testid="event-fields"
            :disabled="busy || readOnly"
          >
            <LocalizedTextFields
              v-model="selected.title"
              :locale="locale"
              :label="copy.eventTitle"
              test-id="event-title"
            />
            <LocalizedTextFields
              v-model="selected.city"
              :locale="locale"
              :label="copy.city"
              test-id="event-city"
            />
            <LocalizedTextFields
              v-model="selected.venue"
              :locale="locale"
              :label="copy.venue"
              test-id="event-venue"
            />
            <div class="music-field-grid">
              <label><span>{{ copy.dateTime }}</span><input
                v-model="selected.dateTime"
                type="text"
                spellcheck="false"
                :data-snapshot-path="dateTimeIssueAttrs['data-snapshot-path']"
                :aria-invalid="dateTimeIssueAttrs['aria-invalid']"
                :aria-describedby="dateTimeIssue ? `event-timezone-hint ${issueId}` : 'event-timezone-hint'"
                data-testid="event-datetime"
              >
                <small id="event-timezone-hint">{{ copy.timezone }}</small>
                <small
                  v-if="dateTimeIssue"
                  :id="issueId"
                  class="field-issue-hint"
                >{{ dateTimeIssueAttrs.description }}</small></label>
              <label><span>{{ copy.status }}</span><select
                v-model="selected.status"
                data-testid="event-status"
              ><option value="scheduled">{{ copy.scheduled }}</option><option value="sold-out">{{ copy.soldOut }}</option><option value="cancelled">{{ copy.cancelled }}</option></select></label>
              <label><span>{{ copy.detailUrl }}</span><input
                v-model="selected.detailUrl"
                type="url"
                spellcheck="false"
                placeholder="https://"
                :data-snapshot-path="detailUrlIssueAttrs['data-snapshot-path']"
                :aria-invalid="detailUrlIssueAttrs['aria-invalid']"
                :aria-describedby="detailUrlIssueAttrs['aria-describedby']"
                data-testid="event-detail-url"
              >
                <small
                  v-if="detailUrlIssue"
                  :id="`${issueId}-detail-url`"
                  class="field-issue-hint"
                >{{ detailUrlIssueAttrs.description }}</small></label>
              <label><span>{{ copy.poster }}</span><select
                :value="selected.posterAssetId || ''"
                data-testid="event-poster"
                @change="setPosterAsset"
              ><option value="">{{ copy.none }}</option><option
                v-for="asset in posters"
                :key="asset.id"
                :value="asset.id"
              >{{ asset.alt[locale] || asset.alt['zh-CN'] }} · {{ asset.id }}</option></select></label>
            </div>
          </fieldset>
        </div>
      </div>
      <div class="music-apply-row">
        <button
          type="button"
          class="button button--primary"
          data-testid="event-apply"
          :disabled="busy || readOnly || stale || !dirty"
          @click="emit('apply')"
        >
          {{ copy.apply }}
        </button>
      </div>
    </template>
  </section>
</template>
