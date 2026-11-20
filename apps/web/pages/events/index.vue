<script setup lang="ts">
import { useHead } from '#imports'
import EventCatalog from '../../components/event/EventCatalog.vue'
import LocaleNotice from '../../components/site/LocaleNotice.vue'
import { useContentSnapshot } from '../../composables/useContentSnapshot'
import { useLocale } from '../../composables/useLocale'
import { eventCatalogMetadata } from '../../utils/event-catalog'

const snapshot = useContentSnapshot()
const { locale, selectLocale } = useLocale()

useHead(() => {
  const { title, description, canonical } = eventCatalogMetadata(snapshot, locale.value)
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
  <EventCatalog
    :snapshot="snapshot"
    :locale="locale"
    @toggle-locale="selectLocale(locale === 'en' ? 'zh-CN' : 'en')"
  />
  <LocaleNotice :raised="false" />
</template>
