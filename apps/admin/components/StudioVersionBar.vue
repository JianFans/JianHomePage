<script setup lang="ts">
import { CopyPlus, History } from '@lucide/vue'
import { computed } from 'vue'
import type { AdminVersion, AdminVersionSummary } from '../utils/admin-api'
import type { AdminLocale } from '../utils/admin-locale'
import { studioCopy } from '../utils/studio-copy'

const props = defineProps<{ version: AdminVersion | null; versions: AdminVersionSummary[]; locale: AdminLocale; busy: boolean; canClone: boolean }>()
const emit = defineEmits<{ select: [id: string]; clone: []; history: [] }>()
const copy = computed(() => studioCopy(props.locale))
const selectable = computed(() => {
  const current = props.version
  return current && !props.versions.some(item => item.id === current.id)
    ? [...props.versions, current]
    : props.versions
})
const visible = computed(() => {
  const recent = selectable.value.slice(0, 4)
  if (props.version && !recent.some(item => item.id === props.version!.id)) recent[3] = props.version
  return recent
})

/** 选择只发出切换意图；取消确认或载入失败时仍显示已加载版本。 */
function selectVersion(event: Event) {
  const select = event.target as HTMLSelectElement
  const requested = select.value
  select.value = props.version?.id || ''
  emit('select', requested)
}
</script>

<template>
  <div class="studio-version-bar">
    <div
      class="studio-version-tabs"
      :aria-label="copy.versions"
    >
      <button
        v-for="item in visible"
        :key="item.id"
        type="button"
        :data-testid="`version-${item.id}`"
        :aria-pressed="version?.id === item.id"
        :disabled="busy"
        @click="emit('select', item.id)"
      >
        <span
          class="studio-status-dot"
          :data-status="item.status"
          aria-hidden="true"
        /><span>{{ item.name || item.id }}</span><small>{{ copy.statuses[item.status] }}</small>
      </button>
      <span
        v-if="!visible.length"
        class="studio-muted"
      >{{ copy.noVersion }}</span>
    </div>
    <select
      class="studio-version-select"
      :aria-label="copy.versions"
      :value="version?.id || ''"
      :disabled="busy || !selectable.length"
      @change="selectVersion"
    >
      <option
        v-if="!selectable.length"
        value=""
      >
        {{ copy.noVersion }}
      </option>
      <option
        v-for="item in selectable"
        :key="item.id"
        :value="item.id"
      >
        {{ item.name || item.id }} · {{ copy.statuses[item.status] }}
      </option>
    </select>
    <div class="studio-version-actions">
      <button
        type="button"
        class="icon-tool"
        :aria-label="copy.versions"
        :title="copy.versions"
        @click="emit('history')"
      >
        <History
          :size="17"
          aria-hidden="true"
        />
      </button>
      <button
        type="button"
        class="button studio-clone"
        :disabled="busy || !canClone || !version"
        data-testid="version-clone"
        :aria-label="copy.clone"
        @click="emit('clone')"
      >
        <CopyPlus
          :size="16"
          aria-hidden="true"
        /><span>{{ copy.clone }}</span>
      </button>
    </div>
  </div>
</template>
