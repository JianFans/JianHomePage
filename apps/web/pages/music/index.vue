<script setup lang="ts">
import { useHead } from '#imports'
import MusicCatalog from '../../components/music/MusicCatalog.vue'
import LocaleNotice from '../../components/site/LocaleNotice.vue'
import { useAudioPlayer } from '../../composables/useAudioPlayer'
import { useContentSnapshot } from '../../composables/useContentSnapshot'
import { useLocale } from '../../composables/useLocale'
import { musicCatalogMetadata } from '../../utils/music-catalog'

const snapshot = useContentSnapshot()
const { locale, selectLocale } = useLocale()
const player = useAudioPlayer()

useHead(() => {
  const { title, description, canonical } = musicCatalogMetadata(snapshot, locale.value)
  return {
    htmlAttrs: { lang: locale.value },
    title,
    meta: [
      { name: 'description', content: description },
      { property: 'og:title', content: title },
      { property: 'og:description', content: description },
      { property: 'og:url', content: canonical },
    ],
    link: [{ rel: 'canonical', href: canonical }],
  }
})
</script>

<template>
  <MusicCatalog
    :snapshot="snapshot"
    :locale="locale"
    :player="player"
    @toggle-locale="selectLocale(locale === 'en' ? 'zh-CN' : 'en')"
  />
  <LocaleNotice :raised="Boolean(player.current.value)" />
</template>
