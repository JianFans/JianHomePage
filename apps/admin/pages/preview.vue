<script setup lang="ts">
import { useHead } from '#imports'
import type { YujianContentSnapshot } from '@yujian/schema'
import { onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import '../../web/assets/css/main.css'
import HomepageView from '../../web/components/home/HomepageView.vue'
import AudioDock from '../../web/components/player/AudioDock.vue'
import { createAudioPlayerController } from '../../web/utils/audio-player'
import type { AdminLocale } from '../utils/admin-locale'
import { createLocaleMessage, createReadyMessage, readPreviewMessage } from '../utils/homepage-preview'

const snapshot = shallowRef<YujianContentSnapshot | null>(null)
const locale = ref<AdminLocale>('zh-CN')
const generation = ref(0)
const player = createAudioPlayerController()
const documentRoot = ref<HTMLElement | null>(null)

useHead(() => ({ htmlAttrs: { lang: locale.value } }))

/** Trust only the embedding parent at this origin, never siblings or direct-open messages. */
function receive(event: MessageEvent) {
  if (window.parent === window) return
  const message = readPreviewMessage(event, location.origin, window.parent)
  if (message?.type === 'snapshot') {
    releaseMedia()
    generation.value++
    snapshot.value = message.snapshot
    locale.value = message.locale
  } else if (message?.type === 'locale') {
    locale.value = message.locale
  }
}

/** Keep the shared header interactive without invoking public-site persistence or routing. */
function toggleLocale() {
  locale.value = locale.value === 'zh-CN' ? 'en' : 'zh-CN'
  if (window.parent !== window) window.parent.postMessage(createLocaleMessage(locale.value), location.origin)
}

/** Prevent external navigation in the capture phase while preserving real in-document anchors. */
function preventNavigation(event: MouseEvent) {
  const anchor = event.target instanceof Element ? event.target.closest('a[href]') : null
  if (!anchor || anchor.getAttribute('href')?.startsWith('#')) return
  event.preventDefault()
  event.stopImmediatePropagation()
}

/** Release media and content while a document is hidden, including entry into the back/forward cache. */
function suspend() {
  window.removeEventListener('message', receive)
  releaseMedia()
  snapshot.value = null
}

/** Removing a video element alone need not stop playback; explicitly release each owned source. */
function releaseMedia() {
  player.close()
  for (const video of documentRoot.value?.querySelectorAll('video') ?? []) {
    video.pause()
    video.removeAttribute('src')
    video.load()
  }
}

/** Restore the handshake after a cached document returns so the host supplies fresh content. */
function resume(event?: PageTransitionEvent) {
  if (event && !event.persisted) return
  window.addEventListener('message', receive)
  if (window.parent !== window) window.parent.postMessage(createReadyMessage(), location.origin)
}

/** End the cache-resume lifecycle as well as media ownership when Vue removes this page. */
function dispose() {
  suspend()
  window.removeEventListener('pagehide', suspend)
  window.removeEventListener('pageshow', resume)
}

onMounted(() => {
  window.addEventListener('pagehide', suspend)
  window.addEventListener('pageshow', resume)
  resume()
})
onBeforeUnmount(dispose)
</script>

<template>
  <div
    ref="documentRoot"
    class="homepage-preview-document"
    :class="{ 'preview--audio-active': player.current.value }"
    @click.capture="preventNavigation"
    @auxclick.capture="preventNavigation"
  >
    <template v-if="snapshot">
      <HomepageView
        :key="generation"
        :snapshot="snapshot"
        :locale="locale"
        :player="player"
        :autoplay-enabled="false"
        @toggle-locale="toggleLocale"
      />
      <AudioDock
        :player="player"
        :locale="locale"
      />
    </template>
    <p
      v-else
      class="preview-waiting"
      role="status"
    >
      {{ locale === 'en' ? 'Waiting for preview content…' : '等待预览内容…' }}
    </p>
  </div>
</template>

<style scoped>
.preview--audio-active { padding-bottom: 6.5rem; }
.preview-waiting { padding: 2rem; color: var(--color-muted); font-size: .9rem; }
</style>
