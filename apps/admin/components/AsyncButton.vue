<script setup lang="ts">
import ActivityIndicator from '../../web/components/ui/ActivityIndicator.vue'

const props = withDefaults(defineProps<{ loading?: boolean; disabled?: boolean; type?: 'button' | 'submit' | 'reset' }>(), { type: 'button' })
const emit = defineEmits<{ click: [event: MouseEvent] }>()

/** 保留原生按钮语义，重复激活不会越过调用者的禁用条件。 */
function activate(event: MouseEvent) {
  if (!props.loading && !props.disabled) emit('click', event)
}
</script>

<template>
  <button
    :type="type"
    class="async-button"
    :disabled="disabled || loading"
    :aria-busy="loading || undefined"
    @click="activate"
  >
    <span class="async-button__content"><slot /></span>
    <span
      v-if="loading"
      class="async-button__indicator"
      aria-hidden="true"
    ><ActivityIndicator /></span>
  </button>
</template>

<style scoped>
.async-button { position: relative; }
.async-button__content { display: inline-flex; align-items: center; justify-content: center; gap: .5rem; min-width: 0; }
.async-button[aria-busy="true"] > .async-button__content { opacity: 0; }
.async-button__indicator { position: absolute; inset: 0; display: grid; place-items: center; font-size: 1.15rem; pointer-events: none; }
.async-button:disabled[aria-busy="true"] { opacity: .8; cursor: progress; }
@media (prefers-reduced-motion: no-preference) {
  .async-button__content { transition: opacity var(--motion-fast, 140ms) var(--motion-ease-out, ease-out); }
}
</style>
