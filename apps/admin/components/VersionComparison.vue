<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, watch } from 'vue'
import { createAdminApi, type AdminVersion, type AdminVersionSummary } from '../utils/admin-api'
import type { AdminLocale } from '../utils/admin-locale'
import { diffSnapshots, formatDiffValue, type SnapshotChangeKind } from '../utils/snapshot-diff'
import AsyncButton from './AsyncButton.vue'

const props = defineProps<{ versions: AdminVersionSummary[]; currentVersionId?: string; apiBaseUrl: string; csrfToken?: string; locale: AdminLocale; active: boolean; nextCursor?: string; listing?: boolean }>()
const emit = defineEmits<{ unauthorized: []; more: [] }>()
interface Side {
  key: 'left' | 'right'
  id: string
  summary?: AdminVersionSummary
  data: AdminVersion | null
  loading: boolean
  error: boolean
  sequence: number
  controller?: AbortController
}
const sides = reactive<Side[]>(['left', 'right'].map(key => ({ key: key as Side['key'], id: '', data: null, loading: false, error: false, sequence: 0 })))
const visibleCounts = reactive<Record<string, number>>({})
const expanded = ref<Record<string, boolean>>({})
const copy = computed(() => props.locale === 'en' ? {
  title: 'Compare saved versions', left: 'Before version', right: 'After version', choose: 'Choose a version', loading: 'Loading saved version…', error: 'Could not load this version.', retry: 'Retry', more: 'Load more versions', empty: 'Choose two saved versions to compare.', noChanges: 'No differences', before: 'Before', after: 'After', value: 'Expand value', limited: 'Large values are shortened for display.', moreChanges: 'Show more changes',
} : {
  title: '对比已保存版本', left: '变更前版本', right: '变更后版本', choose: '选择版本', loading: '正在读取已保存版本…', error: '无法读取此版本。', retry: '重试', more: '加载更多版本', empty: '选择两个已保存版本进行对比。', noChanges: '没有差异', before: '变更前', after: '变更后', value: '展开值', limited: '大字段显示经过截短。', moreChanges: '显示更多差异',
})
const groupNames = computed<Record<string, string>>(() => props.locale === 'en' ? {
  site: 'Site', artist: 'Artist', heroSlides: 'Hero slides', releases: 'Releases', tracks: 'Tracks', videos: 'Videos', events: 'Events', moments: 'Moments', assets: 'Assets', homepage: 'Homepage',
} : { site: '站点资料', artist: '艺人', heroSlides: '宣传位', releases: '作品', tracks: '曲目', videos: '影像', events: '现场', moments: '片段', assets: '素材', homepage: '首页编排' })
const kinds = computed<Record<SnapshotChangeKind, string>>(() => props.locale === 'en'
  ? { added: 'Added', removed: 'Removed', changed: 'Modified', reordered: 'Reordered' }
  : { added: '新增', removed: '删除', changed: '修改', reordered: '排序变化' })
const ready = computed(() => props.active && sides.every(side => side.data && !side.loading && !side.error))
const groups = computed(() => ready.value ? diffSnapshots(sides[0]!.data!.snapshot, sides[1]!.data!.snapshot) : [])

/** 撤销旧读取的写入资格，同时清除对应侧的临时数据。 */
function invalidate(side: Side) {
  side.sequence++
  side.controller?.abort()
  side.controller = undefined
  side.loading = false
  side.error = false
  side.data = null
}

/** 两侧独立读取，只有当前连接与选择仍匹配的响应才可呈现或使会话失效。 */
async function load(side: Side) {
  invalidate(side)
  side.summary = props.versions.find(version => version.id === side.id) || (side.summary?.id === side.id ? side.summary : undefined)
  if (!props.active || !side.id) return
  const sequence = side.sequence
  const id = side.id
  const controller = new AbortController()
  side.controller = controller
  side.loading = true
  const current = () => props.active && side.sequence === sequence && side.id === id && !controller.signal.aborted
  const api = createAdminApi({ baseUrl: props.apiBaseUrl, csrfToken: props.csrfToken, onUnauthorized: () => { if (current()) emit('unauthorized') } })
  try {
    const version = await api.getVersion(id, controller.signal)
    if (!current()) return
    if (version.id !== id || !version.snapshot || typeof version.snapshot !== 'object' || Array.isArray(version.snapshot)) throw new Error('Invalid saved version')
    side.data = version
    side.summary = { id: version.id, name: version.name, revision: version.revision, status: version.status, checksum: version.checksum }
  } catch {
    if (current()) side.error = true
  } finally {
    if (current()) { side.loading = false; side.controller = undefined }
  }
}

/** 仅改变比较侧选择，不触碰编辑器当前版本。 */
function select(side: Side, event: Event) {
  side.id = (event.target as HTMLSelectElement).value
  void load(side)
}

/** 首次进入优先比较当前保存版本与另一个版本，不覆盖已有选择。 */
function defaults() {
  if (!sides[0]!.id) sides[0]!.id = props.versions.find(version => version.id === props.currentVersionId)?.id || props.versions[0]?.id || ''
  if (!sides[1]!.id) sides[1]!.id = props.versions.find(version => version.id !== sides[0]!.id)?.id || props.versions[0]?.id || ''
}

/** 列表刷新未返回旧选择时，保留该保存版本的可见选项。 */
function choices(side: Side) {
  return side.summary && !props.versions.some(version => version.id === side.summary!.id)
    ? [...props.versions, side.summary]
    : props.versions
}

watch(() => [props.active, props.apiBaseUrl, props.csrfToken], () => {
  for (const side of sides) invalidate(side)
  defaults()
  if (props.active) for (const side of sides) void load(side)
}, { immediate: true, flush: 'sync' })
watch(() => props.versions.map(version => `${version.id}:${version.revision}`).join('|'), () => {
  defaults()
  for (const side of sides) {
    const summary = props.versions.find(version => version.id === side.id)
    if (props.active && side.id && !side.loading && (!side.data || (summary && summary.revision !== side.data.revision))) void load(side)
  }
})
watch(groups, () => {
  for (const key of Object.keys(visibleCounts)) delete visibleCounts[key]
  expanded.value = {}
})
onBeforeUnmount(() => { for (const side of sides) invalidate(side) })

/** 按字段及比较侧独立记录展开状态，避免展示全部大值。 */
function toggleValue(key: string, event: Event) {
  expanded.value[key] = (event.target as HTMLDetailsElement).open
}
/** 比较值的标识包含类别、稳定路径及侧别。 */
const valueKey = (group: string, path: string, side: string) => `${group}:${path}:${side}`
/** 折叠状态只显示短预览，展开后仍遵守序列化预算。 */
function preview(value: unknown) { const text = formatDiffValue(value); return text.length > 180 ? `${text.slice(0, 180)}…` : text }
</script>

<template>
  <section
    v-if="active"
    class="version-comparison"
    :aria-label="copy.title"
    data-testid="version-comparison"
  >
    <h3>{{ copy.title }}</h3>
    <div class="comparison-selectors">
      <div
        v-for="side in sides"
        :key="side.key"
        class="comparison-side"
      >
        <label :aria-busy="side.loading || undefined">
          <span>{{ copy[side.key] }}</span>
          <select
            :data-testid="`compare-${side.key}-select`"
            :value="side.id"
            @change="select(side, $event)"
          >
            <option value="">{{ copy.choose }}</option>
            <option
              v-for="version in choices(side)"
              :key="version.id"
              :value="version.id"
            >{{ version.name || version.id }} · r{{ version.revision }}</option>
          </select>
        </label>
        <p
          v-if="side.loading"
          role="status"
        >
          {{ copy.loading }}
        </p>
        <div
          v-else-if="side.error"
          :data-testid="`compare-${side.key}-error`"
          role="alert"
        >
          <p>{{ copy.error }}</p>
          <AsyncButton
            :data-testid="`compare-${side.key}-retry`"
            @click="load(side)"
          >
            {{ copy.retry }}
          </AsyncButton>
        </div>
        <p
          v-else-if="side.data"
          :data-testid="`compare-${side.key}-loaded`"
        >
          {{ side.data.name || side.data.id }} · r{{ side.data.revision }}
        </p>
      </div>
    </div>
    <AsyncButton
      v-if="nextCursor"
      data-testid="compare-more"
      :loading="listing"
      @click="emit('more')"
    >
      {{ copy.more }}
    </AsyncButton>
    <p
      v-if="!sides.every(side => side.id)"
      class="studio-muted"
    >
      {{ copy.empty }}
    </p>
    <p
      v-else-if="ready && !groups.length"
      role="status"
    >
      {{ copy.noChanges }}
    </p>
    <details
      v-for="group in groups"
      :key="group.group"
      class="comparison-group"
      open
    >
      <summary>{{ groupNames[group.group] || group.group }} · {{ group.changes.length }}</summary>
      <ol class="comparison-changes">
        <li
          v-for="change in group.changes.slice(0, visibleCounts[group.group] || 50)"
          :key="`${change.kind}:${change.path}`"
          data-testid="compare-change"
        >
          <div class="comparison-change-title">
            <span :data-kind="change.kind">{{ kinds[change.kind] }}</span><code>{{ change.path }}</code>
          </div>
          <div class="comparison-values">
            <div
              v-for="side in ['before', 'after'] as const"
              :key="side"
              class="comparison-value"
            >
              <strong>{{ copy[side] }}</strong>
              <code v-if="change[`${side}Path`]">{{ change[`${side}Path`] }}</code>
              <details
                v-if="formatDiffValue(change[side]).length > 180 || (change[side] !== null && typeof change[side] === 'object')"
                data-testid="compare-value-details"
                @toggle="toggleValue(valueKey(group.group, change.path, side), $event)"
              >
                <summary>{{ copy.value }} · {{ preview(change[side]) }}</summary>
                <template v-if="expanded[valueKey(group.group, change.path, side)]">
                  <pre>{{ formatDiffValue(change[side]) }}</pre>
                  <small class="studio-muted">{{ copy.limited }}</small>
                </template>
              </details>
              <pre v-else>{{ formatDiffValue(change[side]) }}</pre>
            </div>
          </div>
        </li>
      </ol>
      <button
        v-if="group.changes.length > (visibleCounts[group.group] || 50)"
        type="button"
        data-testid="compare-changes-more"
        @click="visibleCounts[group.group] = (visibleCounts[group.group] || 50) + 50"
      >
        {{ copy.moreChanges }}
      </button>
    </details>
  </section>
</template>

<style scoped>
.version-comparison { min-width: 0; display: grid; gap: 1rem; }
.comparison-selectors, .comparison-values { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1rem; }
.comparison-side, .comparison-side label, .comparison-value { min-width: 0; display: grid; gap: .5rem; align-content: start; }
.comparison-side select { width: 100%; min-height: 44px; }
.comparison-group { border-top: 1px solid var(--line, #333); padding-top: .8rem; min-width: 0; }
summary { cursor: pointer; min-height: 44px; overflow-wrap: anywhere; }
.comparison-changes { display: grid; gap: 1rem; list-style: none; padding: 0; }
.comparison-changes li { min-width: 0; border-bottom: 1px solid var(--line, #333); padding-bottom: 1rem; }
.comparison-change-title { display: flex; flex-wrap: wrap; gap: .75rem; margin-bottom: .75rem; }
code, pre { white-space: pre-wrap; overflow-wrap: anywhere; word-break: break-word; max-width: 100%; margin: 0; }
.comparison-value > code { font-size: .8em; opacity: .75; }
button { min-height: 44px; }
@media (max-width: 640px) { .comparison-selectors, .comparison-values { grid-template-columns: minmax(0, 1fr); } }
</style>
