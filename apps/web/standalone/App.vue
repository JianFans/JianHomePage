<script setup lang="ts">
import type { YujianContentSnapshot } from '@yujian/schema'
import { computed, onMounted, ref } from 'vue'
import HomepageView from '../components/home/HomepageView.vue'
import AudioDock from '../components/player/AudioDock.vue'
import { createAudioPlayerController } from '../utils/audio-player'
import { detectLocale, readLocalePreference, writeLocalePreference } from '../composables/useLocale'
import type { SupportedLocale } from '../utils/localized'
import en from '../locales/en'
import zhCN from '../locales/zh-CN'

const props = defineProps<{ snapshot: YujianContentSnapshot }>()
const locale = ref<SupportedLocale>(props.snapshot.site.defaultLocale)
const player = createAudioPlayerController()
const notice = ref(false)
const noticeText = computed(() => (locale.value === 'en' ? en : zhCN).locale.automatic)

/** Persist only after hydration, so server markup always matches the initial browser render. */
function selectLocale(value: SupportedLocale) {
  locale.value = value
  notice.value = false
  document.documentElement.lang = value
  writeLocalePreference(undefined, value)
}
onMounted(() => {
  const detected = detectLocale({ stored: readLocalePreference(), browser: navigator.languages.length ? navigator.languages : [navigator.language] })
  selectLocale(detected.locale)
  notice.value = detected.notify
})
</script>

<template>
  <main :class="{ 'main--audio-active': player.current.value }">
    <HomepageView
      :snapshot="snapshot"
      :locale="locale"
      :player="player"
      @toggle-locale="selectLocale(locale === 'en' ? 'zh-CN' : 'en')"
    />
  </main>
  <div data-testid="audio-dock-host">
    <AudioDock
      :player="player"
      :locale="locale"
    />
  </div>
  <Transition name="locale-notice">
    <div
      v-if="notice"
      class="standalone-locale-notice"
      :class="{ raised: player.current.value }"
      role="status"
      aria-live="polite"
    >
      <span>{{ noticeText }}</span>
      <button
        type="button"
        :aria-label="locale === 'en' ? 'Close' : '关闭'"
        @click="notice = false"
      >
        ×
      </button>
    </div>
  </Transition>
</template>

<style>
.main--audio-active { padding-bottom: 6.5rem; }
.standalone-locale-notice { position: fixed; z-index: 50; right: 1rem; bottom: 1rem; display: flex; align-items: center; gap: .75rem; max-width: calc(100vw - 2rem); padding: .5rem 1rem; border: 1px solid var(--color-border); background: var(--color-surface-raised); font-size: .8rem; }
.standalone-locale-notice.raised { bottom: 6.5rem; }
.standalone-locale-notice button { min-width: 2.75rem; min-height: 2.75rem; border: 0; background: transparent; color: inherit; font-size: 1.5rem; }
.locale-notice-enter-active, .locale-notice-leave-active { transition: opacity 160ms, transform 160ms; }
.locale-notice-enter-from, .locale-notice-leave-to { opacity: 0; transform: translateY(.5rem); }
@media (prefers-reduced-motion: reduce) { .locale-notice-enter-active, .locale-notice-leave-active { transition: none; } }
</style>
