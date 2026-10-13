<script setup lang="ts">
import { useHead } from '#imports'
import HomepageView from '../components/home/HomepageView.vue'
import LocaleNotice from '../components/site/LocaleNotice.vue'
import { useAudioPlayer } from '../composables/useAudioPlayer'
import { useContentSnapshot } from '../composables/useContentSnapshot'
import { useLocale } from '../composables/useLocale'
import {
  buildStructuredData,
  serializeStructuredData,
} from '../utils/structured-data'

const snapshot = useContentSnapshot()
const { locale, selectLocale } = useLocale()
const player = useAudioPlayer()
const structuredData = serializeStructuredData(buildStructuredData(snapshot, 'zh-CN'))

useHead(() => ({
  htmlAttrs: {
    lang: locale.value,
  },
  script: [{
    key: 'homepage-structured-data',
    type: 'application/ld+json',
    innerHTML: structuredData,
  }],
}))

/** Keep locale persistence in the public host; the shared view only emits intent. */
function toggleLocale() {
  selectLocale(locale.value === 'zh-CN' ? 'en' : 'zh-CN')
}
</script>

<template>
  <HomepageView
    :snapshot="snapshot"
    :locale="locale"
    :player="player"
    @toggle-locale="toggleLocale"
  />
  <LocaleNotice :raised="Boolean(player.current.value)" />
</template>
