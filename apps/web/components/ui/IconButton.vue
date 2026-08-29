<script setup lang="ts">
import { useId } from 'vue'

defineOptions({ inheritAttrs: false })

withDefaults(defineProps<{
  label: string
  disabled?: boolean
  pressed?: boolean
  tooltipAlign?: 'center' | 'end'
}>(), {
  disabled: false,
  pressed: undefined,
  tooltipAlign: 'center',
})

const emit = defineEmits<{
  click: [event: MouseEvent]
}>()
const tooltipId = `tooltip-${useId()}`

function handleClick(event: MouseEvent) {
  emit('click', event)
}
</script>

<template>
  <span class="icon-button-shell">
    <button
      v-bind="$attrs"
      type="button"
      class="icon-button"
      :aria-label="label"
      :aria-describedby="tooltipId"
      :aria-pressed="pressed"
      :disabled="disabled"
      @click="handleClick"
    >
      <slot />
    </button>
    <span
      :id="tooltipId"
      class="icon-button-tooltip"
      :class="{ 'icon-button-tooltip--end': tooltipAlign === 'end' }"
      role="tooltip"
    >
      {{ label }}
    </span>
  </span>
</template>

<style scoped>
.icon-button-shell {
  position: relative;
  display: inline-grid;
  width: 2.75rem;
  height: 2.75rem;
  flex: 0 0 2.75rem;
}

.icon-button {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  padding: 0;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--color-border) 72%, transparent);
  border-radius: var(--radius-tool);
  background: color-mix(in srgb, var(--color-bg) 72%, transparent);
  color: var(--color-text);
  cursor: pointer;
  transition: background-color 160ms var(--ease-standard), border-color 160ms var(--ease-standard), color 160ms var(--ease-standard);
}

.icon-button :deep(svg) {
  width: 1.125rem;
  height: 1.125rem;
  stroke-width: 1.75;
}

.icon-button:hover,
.icon-button:focus-visible,
.icon-button[aria-pressed="true"] {
  border-color: var(--color-accent);
  background: var(--color-surface-raised);
  color: var(--color-focus);
}

.icon-button:disabled {
  opacity: 0.38;
  cursor: not-allowed;
}

.icon-button-tooltip {
  position: absolute;
  z-index: 40;
  top: calc(100% + 0.5rem);
  left: 50%;
  width: max-content;
  max-width: 12rem;
  padding: 0.35rem 0.5rem;
  transform: translateX(-50%) translateY(-0.15rem);
  border: 1px solid var(--color-border);
  border-radius: 3px;
  background: var(--color-surface-raised);
  color: var(--color-text);
  font-size: 0.75rem;
  line-height: 1.2;
  opacity: 0;
  pointer-events: none;
  transition: opacity 140ms var(--ease-standard), transform 140ms var(--ease-standard);
}

.icon-button:hover + .icon-button-tooltip,
.icon-button:focus-visible + .icon-button-tooltip {
  transform: translateX(-50%) translateY(0);
  opacity: 1;
}

.icon-button-tooltip--end {
  right: 0;
  left: auto;
  transform: translateY(-0.15rem);
}

.icon-button:hover + .icon-button-tooltip--end,
.icon-button:focus-visible + .icon-button-tooltip--end {
  transform: translateY(0);
}
</style>
