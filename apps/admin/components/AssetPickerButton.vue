<script setup lang="ts">
import { inject, onBeforeUnmount, ref } from 'vue'
import { FolderOpen } from '@lucide/vue'
import type { Asset } from '@yujian/schema'
import { assetPickerKey } from '../utils/asset-selection'

const props = defineProps<{ kinds: readonly Asset['kind'][]; label: string; disabled?: boolean }>()
const emit = defineEmits<{ select: [id: string] }>()
const picker = inject(assetPickerKey, null)
const waiting = ref(false)
let alive = true
onBeforeUnmount(() => { alive = false })
/** 宿主负责上下文校验；组件卸载后的结果不能写入已切走的表单。 */
async function browse(event: MouseEvent) {
  if (!picker || props.disabled || waiting.value || (event.currentTarget as HTMLElement).closest('fieldset:disabled')) return
  waiting.value = true
  try {
    const asset = await picker({ kinds: props.kinds, label: props.label })
    if (asset && alive && !props.disabled) emit('select', asset.id)
  } finally { waiting.value = false }
}
</script>

<template>
  <button
    v-if="picker"
    type="button"
    class="button asset-picker-trigger"
    :disabled="disabled"
    :aria-expanded="waiting"
    aria-haspopup="dialog"
    :aria-label="label"
    :title="label"
    data-testid="asset-picker-open"
    @click="browse"
  >
    <FolderOpen
      :size="16"
      aria-hidden="true"
    />
  </button>
</template>

<style scoped>
.asset-picker-trigger { min-height: 2.75rem; min-width: 2.75rem; justify-self: start; }
</style>

<style>
.asset-field { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-content: start; align-items: center; gap: .5rem; min-width: 0; }
.asset-field > label { grid-column: 1; }
.asset-field > .asset-picker-trigger { grid-column: 2; grid-row: 1; }
.asset-field > select, .asset-field > small { grid-column: 1 / -1; min-width: 0; }
</style>
