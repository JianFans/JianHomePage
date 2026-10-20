<script setup lang="ts">
import { Check, CircleAlert, Info, TriangleAlert, X } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted } from 'vue'
import type { ToastMessage } from '../composables/useToasts'
import type { AdminLocale } from '../utils/admin-locale'

const props = defineProps<{ items: readonly ToastMessage[]; locale: AdminLocale }>()
const emit = defineEmits<{
  dismiss: [id: string]
  pause: [id: string, reason: string]
  resume: [id: string, reason: string]
  visibility: [hidden: boolean]
}>()
const icons = { success: Check, info: Info, warning: TriangleAlert, error: CircleAlert }
const closeLabel = computed(() => props.locale === 'en' ? 'Dismiss message' : '关闭提示')

/** 聚焦在同一条消息内移动时继续暂停，离开整条消息后才恢复。 */
function focusOut(id: string, event: FocusEvent) {
  if (!(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node | null)) emit('resume', id, 'focus')
}
/** 隐藏页面暂停计时，恢复后不丢失消息的剩余展示时间。 */
function visibilityChanged() { emit('visibility', document.hidden) }
onMounted(() => { document.addEventListener('visibilitychange', visibilityChanged); visibilityChanged() })
onBeforeUnmount(() => document.removeEventListener('visibilitychange', visibilityChanged))
</script>

<template>
  <TransitionGroup
    name="toast"
    tag="div"
    class="toast-viewport"
    data-testid="toast-viewport"
  >
    <article
      v-for="item in items"
      :key="item.id"
      class="toast-message"
      :class="`toast-message--${item.tone}`"
      @pointerenter="emit('pause', item.id, 'pointer')"
      @pointerleave="emit('resume', item.id, 'pointer')"
      @focusin="emit('pause', item.id, 'focus')"
      @focusout="focusOut(item.id, $event)"
    >
      <component
        :is="icons[item.tone]"
        :size="18"
        aria-hidden="true"
      />
      <p
        :role="item.tone === 'error' ? 'alert' : 'status'"
        aria-atomic="true"
      >
        {{ item.message }}
      </p>
      <button
        type="button"
        class="icon-tool"
        :aria-label="closeLabel"
        @click="emit('dismiss', item.id)"
      >
        <X
          :size="16"
          aria-hidden="true"
        />
      </button>
    </article>
  </TransitionGroup>
</template>

<style scoped>
.toast-viewport { position: fixed; z-index: 120; bottom: max(1.25rem, env(safe-area-inset-bottom)); right: max(1rem, env(safe-area-inset-right)); display: flex; flex-direction: column; align-items: flex-end; gap: .65rem; pointer-events: none; }
.toast-message { --toast-accent: var(--info, #a7bfd5); box-sizing: border-box; inline-size: max-content; max-inline-size: min(28rem, calc(100vw - 2rem - env(safe-area-inset-left) - env(safe-area-inset-right))); pointer-events: auto; display: grid; grid-template-columns: 1.15rem minmax(0, 1fr) 2.75rem; align-items: center; gap: .7rem; padding: .6rem .6rem .6rem 1rem; color: var(--text); background: var(--surface-raised); border: 1px solid var(--border); border-left: 3px solid var(--toast-accent); border-radius: .7rem; box-shadow: 0 8px 32px #0005; }
.toast-message > svg { color: var(--toast-accent); }
.toast-message p { margin: 0; font-size: .85rem; overflow-wrap: anywhere; }
.toast-message--success { --toast-accent: var(--accent, #9ebdb5); }
.toast-message--warning { --toast-accent: var(--warm, #d2b481); }
.toast-message--error { --toast-accent: var(--danger, #dcaaa2); }
.toast-message .icon-tool { border: 0; border-radius: .4rem; }
.toast-enter-active, .toast-leave-active, .toast-move { transition: opacity 180ms ease, transform 220ms ease; }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(.5rem); }
.toast-leave-active { position: absolute; right: 0; }
@media (prefers-reduced-motion: reduce) { .toast-enter-active, .toast-leave-active, .toast-move { transition: none; } }
</style>
