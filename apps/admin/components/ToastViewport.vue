<script setup lang="ts">
import { Check, CircleAlert, Info, TriangleAlert, X } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, onUpdated } from 'vue'
import type { ToastMessage } from '../composables/useToasts'
import type { AdminLocale } from '../utils/admin-locale'

const props = defineProps<{ items: readonly ToastMessage[]; locale: AdminLocale }>()
const emit = defineEmits<{
  dismiss: [id: string]
  pause: [id: string, reason: string]
  resume: [id: string, reason: string]
  pauseAll: [reason: string]
  resumeAll: [reason: string]
  visibility: [hidden: boolean]
}>()
const icons = { success: Check, info: Info, warning: TriangleAlert, error: CircleAlert }
const closeLabel = computed(() => props.locale === 'en' ? 'Dismiss message' : '关闭提示')

let focusedViewport: HTMLElement | null = null
/** 聚焦视口或任意关闭按钮时暂停全部消息，也覆盖期间新到达的提示。 */
function focusIn(event: FocusEvent) {
  focusedViewport = event.currentTarget as HTMLElement
  emit('pauseAll', 'focus')
}
/** 只释放焦点暂停原因，不影响页面隐藏或单条悬停暂停。 */
function resumeFocus() {
  if (!focusedViewport) return
  focusedViewport = null
  emit('resumeAll', 'focus')
}
/** 焦点在视口内部移动时保持暂停，离开整个区域后才恢复。 */
function focusOut(event: FocusEvent) {
  if (!(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node | null)) resumeFocus()
}
/** 聚焦节点移除后可能没有 focusout；更新及离场结束后核对，避免永久暂停。 */
function recheckFocus() {
  if (focusedViewport && !focusedViewport.contains(document.activeElement)) resumeFocus()
}
onUpdated(recheckFocus)
/** 隐藏页面暂停计时，恢复后不丢失消息的剩余展示时间。 */
function visibilityChanged() { emit('visibility', document.hidden) }
onMounted(() => { document.addEventListener('visibilitychange', visibilityChanged); visibilityChanged() })
onBeforeUnmount(() => { document.removeEventListener('visibilitychange', visibilityChanged); resumeFocus() })
</script>

<template>
  <TransitionGroup
    name="toast"
    tag="div"
    class="toast-viewport"
    data-testid="toast-viewport"
    role="region"
    :aria-label="locale === 'en' ? 'Messages' : '消息提示'"
    :tabindex="items.length ? 0 : -1"
    @focusin="focusIn"
    @focusout="focusOut"
    @after-leave="recheckFocus"
  >
    <article
      v-for="item in items"
      :key="item.id"
      class="toast-message"
      :class="`toast-message--${item.tone}`"
      @pointerenter="emit('pause', item.id, 'pointer')"
      @pointerleave="emit('resume', item.id, 'pointer')"
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
.toast-viewport { position: fixed; z-index: 120; bottom: max(1.25rem, env(safe-area-inset-bottom)); right: max(1rem, env(safe-area-inset-right)); max-block-size: calc(100dvh - max(1.25rem, env(safe-area-inset-bottom)) - max(1rem, env(safe-area-inset-top))); overflow-x: hidden; overflow-y: auto; scrollbar-width: none; display: flex; flex-direction: column; align-items: flex-end; gap: .65rem; pointer-events: none; }
/* 保留滚轮、触控和键盘滚动，避免出入动画的临时溢出闪现滚动条。 */
.toast-viewport::-webkit-scrollbar { display: none; }
.toast-message { --toast-accent: var(--info, #a7bfd5); flex-shrink: 0; box-sizing: border-box; inline-size: max-content; max-inline-size: min(28rem, calc(100vw - 2rem - env(safe-area-inset-left) - env(safe-area-inset-right))); pointer-events: auto; display: grid; grid-template-columns: 1.15rem minmax(0, 1fr) 2.75rem; align-items: center; gap: .7rem; padding: .6rem .6rem .6rem 1rem; color: var(--text); background: var(--surface-raised); border: 1px solid var(--border); border-left: 3px solid var(--toast-accent); border-radius: .7rem; box-shadow: 0 8px 32px #0005; }
.toast-message > svg { color: var(--toast-accent); }
.toast-message p { margin: 0; font-size: .85rem; overflow-wrap: anywhere; }
.toast-message--success { --toast-accent: var(--accent, #9ebdb5); }
.toast-message--warning { --toast-accent: var(--warm, #d2b481); }
.toast-message--error { --toast-accent: var(--danger, #dcaaa2); }
.toast-message .icon-tool { border: 0; border-radius: .4rem; }
.toast-enter-active, .toast-leave-active, .toast-move { transition: opacity var(--motion-normal) var(--motion-ease-out), transform var(--motion-normal) var(--motion-ease-out); }
.toast-enter-from, .toast-leave-to { opacity: 0; transform: translateY(.5rem); }
.toast-leave-active { position: absolute; right: 0; }
@media (prefers-reduced-motion: reduce) { .toast-enter-active, .toast-leave-active, .toast-move { transition: none; } }
</style>
