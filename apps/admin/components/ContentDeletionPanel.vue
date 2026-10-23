<script setup lang="ts">
import { Trash2 } from '@lucide/vue'
import { computed, ref, useId, watch } from 'vue'
import { analyzeContentDeletion, applyContentDeletion, type ContentDeletionErrorCode, type ContentDeletionKind, type ContentDeletionPlan } from '../utils/content-deletion'
import type { AdminLocale } from '../utils/admin-locale'
import type { YujianContentSnapshot } from '@yujian/schema'
import StudioDialog from './StudioDialog.vue'

const props = defineProps<{ text: string; analyzedText: string; snapshot: YujianContentSnapshot | null; kind: ContentDeletionKind; locale: AdminLocale; locked: boolean }>()
const emit = defineEmits<{ 'update:text': [text: string]; notify: [message: string, tone: 'success' | 'error'] }>()
const inputId = useId()
const selected = ref('')
const plan = ref<ContentDeletionPlan | null>(null)
const acknowledged = ref(false)
// 展示复用工作区防抖分析；执行时仍由删除事务重新校验原文。
const current = computed(() => props.text === props.analyzedText)
const copy = computed(() => props.locale === 'en' ? {
  title: 'Delete content', select: 'Content to delete', open: 'Delete…', confirm: 'Confirm deletion', cancel: 'Cancel', close: 'Close',
  ack: 'I confirm deleting these records and removing their listed references.',
  removed: 'Records to delete', references: 'Affected references', none: 'No incoming references.',
  locked: 'Save open forms first. Deletion is unavailable while the workspace is locked or read-only.',
  scope: 'This changes the current snapshot. Save it using the toolbar. History and media files are retained.',
  empty: 'No content available.', success: 'Content removed from the snapshot. Save to keep this change.',
  actions: { arrangement: 'Remove from homepage', ownership: 'Remove from release', cascade: 'Deleted with record', blocking: 'Edit this reference first' },
} : {
  title: '删除内容', select: '选择删除记录', open: '删除…', confirm: '确认删除', cancel: '取消', close: '关闭',
  ack: '我确认删除以上记录，并移除列出的编排及所属作品引用。',
  removed: '将删除的记录', references: '受影响的引用', none: '没有引用此记录的字段。',
  locked: '请先保存已打开的表单。工作区锁定或只读时无法删除。',
  scope: '仅修改当前快照，请通过顶部保存保留修改。历史版本和素材文件会保留。',
  empty: '没有可删除的记录。', success: '已从快照移除内容，请保存以保留修改。',
  actions: { arrangement: '从首页编排移除', ownership: '从所属作品移除', cascade: '随记录一并删除', blocking: '请先修改此引用' },
})
const errors = computed<Record<ContentDeletionErrorCode, string>>(() => props.locale === 'en' ? {
  'invalid-snapshot': 'Fix the snapshot validation errors before deleting content.',
  'missing-target': 'This record no longer exists.', 'last-track': 'The last track in a release cannot be deleted separately.',
  'blocked-reference': 'Other content refers to these records. Edit the listed references first.',
  'stale-snapshot': 'The snapshot changed. Review the deletion again.',
  'invalid-result': 'The resulting snapshot is invalid. No changes were applied.',
} : {
  'invalid-snapshot': '请先修正快照校验问题，再删除内容。', 'missing-target': '此记录已不存在。',
  'last-track': '作品的最后一首曲目不能单独删除。', 'blocked-reference': '其他内容仍引用这些记录，请先修改列出的引用。',
  'stale-snapshot': '快照已改变，请重新检查删除影响。', 'invalid-result': '删除后的快照未通过校验，原文已保留。',
})
const records = computed(() => {
  const snapshot = props.snapshot
  if (!snapshot) return []
  const key = { release: 'releases', track: 'tracks', video: 'videos', event: 'events', moment: 'moments', hero: 'heroSlides' } as const
  return snapshot[key[props.kind]].map(record => {
    const localized = 'title' in record ? record.title : 'headline' in record ? record.headline : 'caption' in record ? record.caption : undefined
    return { id: record.id, label: localized?.[props.locale] || localized?.['zh-CN'] || record.id }
  })
})
const selectedLabel = computed(() => records.value.find(record => record.id === selected.value)?.label || selected.value)
const disabled = computed(() => props.locked || !current.value || !props.snapshot || !selected.value)

/** 任何上下文变化均撤销之前的影响分析与确认勾选。 */
function close() { plan.value = null; acknowledged.value = false }

watch(records, values => {
  if (!values.some(record => record.id === selected.value)) selected.value = values[0]?.id || ''
}, { immediate: true })
watch(() => [props.text, props.kind, props.locked, selected.value], close, { flush: 'sync' })

/** 按下删除时分析最新文本，展示实际会修改的记录与引用。 */
function open() {
  if (disabled.value) return
  acknowledged.value = false
  plan.value = analyzeContentDeletion(props.text, props.kind, selected.value)
}

/** 只有当前确认仍有效且事务完整通过时才交回新快照。 */
function confirm() {
  if (props.locked || !plan.value || !acknowledged.value || plan.value.error) return
  const result = applyContentDeletion(props.text, plan.value)
  if (result.error) {
    emit('notify', errors.value[result.error], 'error')
    close()
    return
  }
  close()
  emit('update:text', result.text)
  emit('notify', copy.value.success, 'success')
}
</script>

<template>
  <section
    class="content-deletion"
    :aria-label="copy.title"
  >
    <label :for="inputId">{{ copy.select }}</label>
    <div class="deletion-controls">
      <select
        :id="inputId"
        v-model="selected"
        :disabled="locked || !records.length"
        data-testid="content-delete-select"
      >
        <option
          v-if="!records.length"
          value=""
        >
          {{ copy.empty }}
        </option>
        <option
          v-for="record in records"
          :key="record.id"
          :value="record.id"
        >
          {{ record.label }}
        </option>
      </select>
      <button
        type="button"
        :disabled="disabled"
        data-testid="content-delete-open"
        @click="open"
      >
        <Trash2
          :size="16"
          aria-hidden="true"
        />{{ copy.open }}
      </button>
    </div>
    <p
      v-if="locked"
      class="deletion-hint"
    >
      {{ copy.locked }}
    </p>
    <p
      v-else-if="current && !snapshot"
      role="status"
    >
      {{ errors['invalid-snapshot'] }}
    </p>
    <StudioDialog
      :open="Boolean(plan)"
      :title="copy.title"
      :close-label="copy.close"
      @close="close"
    >
      <template v-if="plan">
        <p class="deletion-target">
          {{ selectedLabel }}
        </p>
        <p class="deletion-hint">
          {{ copy.scope }}
        </p>
        <h3>{{ copy.removed }}</h3>
        <ul>
          <li
            v-for="record in plan.removed"
            :key="record.path"
          >
            <code>{{ record.path }}</code><small>{{ record.id }}</small>
          </li>
        </ul>
        <h3>{{ copy.references }}</h3>
        <ul v-if="plan.references.length">
          <li
            v-for="reference in plan.references"
            :key="reference.path"
            :class="{ 'deletion-blocked': reference.action === 'blocking' }"
          >
            <code>{{ reference.path }}</code><small>{{ copy.actions[reference.action] }}</small>
          </li>
        </ul>
        <p v-else>
          {{ copy.none }}
        </p>
        <p
          v-if="plan.error"
          role="alert"
        >
          {{ errors[plan.error] }}
        </p>
        <label class="deletion-ack">
          <input
            v-model="acknowledged"
            type="checkbox"
            data-testid="content-delete-ack"
          >
          <span>{{ copy.ack }}</span>
        </label>
        <div class="deletion-actions">
          <button
            type="button"
            @click="close"
          >
            {{ copy.cancel }}
          </button>
          <button
            type="button"
            :disabled="locked || !acknowledged || Boolean(plan.error)"
            data-testid="content-delete-confirm"
            @click="confirm"
          >
            {{ copy.confirm }}
          </button>
        </div>
      </template>
    </StudioDialog>
  </section>
</template>

<style scoped>
.content-deletion { margin-top: 1.5rem; border-top: 1px solid var(--border); padding-top: 1rem; min-width: 0; }
.deletion-controls { display: flex; gap: .65rem; margin-top: .45rem; }
.deletion-controls select { flex: 1; min-width: 0; }
.deletion-controls button { display: inline-flex; align-items: center; justify-content: center; gap: .4rem; flex-shrink: 0; }
.deletion-hint, small { color: var(--muted); font-size: .85rem; }
.deletion-target { overflow-wrap: anywhere; }
h3 { font-size: .9rem; font-weight: 500; margin: 1.25rem 0 .5rem; }
ul { padding-left: 1.1rem; }
li { margin: .55rem 0; }
code, small { display: block; overflow-wrap: anywhere; }
.deletion-blocked, [role="alert"] { color: var(--danger, #e5a8a8); }
.deletion-ack { display: flex; align-items: flex-start; gap: .55rem; margin-top: 1.25rem; }
.deletion-ack input { flex: 0 0 auto; margin-top: .25rem; }
.deletion-actions { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: .65rem; margin-top: 1.25rem; }
button, select { min-height: 2.75rem; }
@media (max-width: 480px) { .deletion-controls { flex-wrap: wrap; } .deletion-controls select { flex-basis: 100%; } }
</style>
