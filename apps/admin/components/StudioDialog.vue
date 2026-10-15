<script setup lang="ts">
import { X } from '@lucide/vue'
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'

const props = defineProps<{ open: boolean; contentKey?: string; title: string; closeLabel: string; busy?: boolean; wide?: boolean }>()
const emit = defineEmits<{ close: [] }>()
const id = useId()
const root = ref<HTMLElement | null>(null)
let previousFocus: HTMLElement | null = null
let previousOverflow = ''
const frameListeners = new Map<HTMLIFrameElement, { document: Document; keydown: (event: KeyboardEvent) => void }>()
let frameObserver: MutationObserver | null = null
const focusableSelector = 'button, input, textarea, select, a[href], iframe, summary, [tabindex]'

/** 每次按键重新读取候选，使动态播放器和消息按钮遵守当前可见顺序。 */
function focusableNodes(scope: HTMLElement | Document | null) {
  return [...(scope?.querySelectorAll<HTMLElement>(focusableSelector) || [])]
    .filter(node => node.tabIndex >= 0 && !node.matches(':disabled') && !node.closest('[hidden], [inert], [aria-hidden="true"]') && node.getClientRects().length > 0)
}

/** 关闭或替换预览时释放子文档监听，不保留旧 iframe。 */
function clearFrameListeners() {
  frameObserver?.disconnect()
  frameObserver = null
  for (const entry of frameListeners.values()) entry.document.removeEventListener('keydown', entry.keydown)
  frameListeners.clear()
}

/** 同源预览的按键不会冒泡到父文档，显式移交首尾 Tab 和 Escape。 */
function syncFrameListeners() {
  if (!props.open) return
  const frames = [...(root.value?.querySelectorAll('iframe') || [])]
  for (const [frame, entry] of frameListeners) {
    if (!frames.includes(frame) || frame.contentDocument !== entry.document) {
      entry.document.removeEventListener('keydown', entry.keydown)
      frameListeners.delete(frame)
    }
  }
  for (const frame of frames) {
    // contentDocument 对跨源页面返回 null；不放宽 iframe 的 sandbox。
    const childDocument = frame.contentDocument
    if (!childDocument || frameListeners.has(frame)) continue
    const listener = (event: KeyboardEvent) => {
      if (!props.open) return
      if (event.key === 'Escape') { event.preventDefault(); if (!props.busy) emit('close'); return }
      if (event.key !== 'Tab') return
      const children = focusableNodes(childDocument)
      const boundary = event.shiftKey ? children[0] : children.at(-1)
      if (children.length && childDocument.activeElement !== boundary) return
      const nodes = focusableNodes(root.value)
      const index = nodes.indexOf(frame)
      if (index < 0) return
      event.preventDefault()
      nodes[(index + (event.shiftKey ? -1 : 1) + nodes.length) % nodes.length]?.focus()
    }
    childDocument.addEventListener('keydown', listener)
    frameListeners.set(frame, { document: childDocument, keydown: listener })
  }
}

/** 锁定背景滚动并把焦点送入弹窗，关闭后恢复触发者。 */
watch(() => [props.open, props.contentKey] as const, async ([open], [wasOpen]) => {
  if (open) {
    if (!wasOpen) {
      previousFocus = document.activeElement as HTMLElement | null
      previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
    }
    await nextTick()
    if (!props.open) return
    frameObserver?.disconnect()
    frameObserver = new MutationObserver(syncFrameListeners)
    if (root.value) frameObserver.observe(root.value, { childList: true, subtree: true })
    syncFrameListeners()
    const target = root.value?.querySelector<HTMLElement>('[autofocus]')
      || root.value?.querySelector<HTMLElement>('button, input, textarea, select, [tabindex="0"]')
    target?.focus()
  } else if (wasOpen) {
    clearFrameListeners()
    document.body.style.overflow = previousOverflow
    previousFocus?.focus()
    previousFocus = null
  }
})

/** Tab 限定在弹窗和仍可操作的消息区；Escape 不打断正在执行的写操作。 */
function keydown(event: KeyboardEvent) {
  if (!props.open) return
  if (event.key === 'Escape') { event.preventDefault(); if (!props.busy) emit('close'); return }
  if (event.key !== 'Tab') return
  const nodes = focusableNodes(root.value)
  if (!nodes.length) { event.preventDefault(); root.value?.focus(); return }
  if (event.shiftKey && document.activeElement === nodes[0]) { event.preventDefault(); nodes.at(-1)?.focus() }
  else if (!event.shiftKey && document.activeElement === nodes.at(-1)) { event.preventDefault(); nodes[0]?.focus() }
}
onBeforeUnmount(() => {
  clearFrameListeners()
  if (props.open) document.body.style.overflow = previousOverflow
})
</script>

<template>
  <div
    ref="root"
    class="studio-overlay-root"
    :role="open ? 'dialog' : undefined"
    :aria-modal="open ? true : undefined"
    :aria-labelledby="open ? id : undefined"
    tabindex="-1"
    @keydown="keydown"
    @load.capture="syncFrameListeners"
  >
    <Transition name="studio-dialog">
      <div
        v-if="open"
        class="studio-dialog-wrap"
      >
        <div
          class="studio-dialog-scrim"
          aria-hidden="true"
          @click="!busy && emit('close')"
        />
        <section
          class="studio-dialog"
          :class="{ 'studio-dialog--wide': wide }"
        >
          <header>
            <h2 :id="id">
              {{ title }}
            </h2><button
              type="button"
              class="icon-tool"
              :aria-label="closeLabel"
              :disabled="busy"
              data-testid="dialog-close"
              @click="emit('close')"
            >
              <X
                :size="18"
                aria-hidden="true"
              />
            </button>
          </header>
          <slot />
        </section>
      </div>
    </Transition>
    <slot name="messages" />
  </div>
</template>

<style scoped>
.studio-overlay-root { position: relative; z-index: 100; }
.studio-dialog-wrap { position: fixed; inset: 0; display: grid; place-items: center; padding: 1.25rem; isolation: isolate; }
.studio-dialog-scrim { position: absolute; inset: 0; background: #070a0dbd; backdrop-filter: blur(5px); z-index: -1; }
.studio-dialog { width: min(32rem, 100%); max-height: calc(100dvh - 2.5rem); overflow: auto; background: var(--surface); border: 1px solid var(--border); border-radius: 1rem; padding: 1.5rem; box-shadow: 0 24px 100px #0007; }
.studio-dialog--wide { width: min(82rem, 100%); }
.studio-dialog > header { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 1.25rem; }
.studio-dialog h2 { margin: 0; font-size: 1.05rem; font-weight: 500; }
.studio-dialog-enter-active, .studio-dialog-leave-active { transition: opacity 180ms ease; }
.studio-dialog-enter-active .studio-dialog, .studio-dialog-leave-active .studio-dialog { transition: transform 220ms ease; }
.studio-dialog-enter-from, .studio-dialog-leave-to { opacity: 0; }
.studio-dialog-enter-from .studio-dialog, .studio-dialog-leave-to .studio-dialog { transform: translateY(.65rem) scale(.99); }
@media (prefers-reduced-motion: reduce) { .studio-dialog-enter-active, .studio-dialog-leave-active, .studio-dialog-enter-active .studio-dialog, .studio-dialog-leave-active .studio-dialog { transition: none; } }
</style>
