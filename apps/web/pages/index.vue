<script setup lang="ts">
import { useHead } from '#imports'
import { computed } from 'vue'
import ArtistSection from '../components/home/ArtistSection.vue'
import EventSection from '../components/home/EventSection.vue'
import HeroShowcase from '../components/home/HeroShowcase.vue'
import MomentSection from '../components/home/MomentSection.vue'
import MusicSection from '../components/home/MusicSection.vue'
import VideoSection from '../components/home/VideoSection.vue'
import LocaleNotice from '../components/site/LocaleNotice.vue'
import SiteFooter from '../components/site/SiteFooter.vue'
import SiteHeader from '../components/site/SiteHeader.vue'
import { useAudioPlayer } from '../composables/useAudioPlayer'
import { useContentSnapshot } from '../composables/useContentSnapshot'
import { useLocale } from '../composables/useLocale'
import { resolveLocalized } from '../utils/localized'
import { resolveHomepageSections } from '../utils/sections'
import {
  buildStructuredData,
  serializeStructuredData,
} from '../utils/structured-data'

const snapshot = useContentSnapshot()
const { locale, selectLocale } = useLocale()
const player = useAudioPlayer()
const sections = computed(() => resolveHomepageSections(snapshot, locale.value))
const brand = computed(() => resolveLocalized(snapshot.site.brand, locale.value))
const artistName = computed(() => resolveLocalized(snapshot.site.artistName, locale.value))
const copyrightYear = new Date(snapshot.generatedAt).getUTCFullYear()
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

function toggleLocale() {
  selectLocale(locale.value === 'zh-CN' ? 'en' : 'zh-CN')
}
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    @toggle-locale="toggleLocale"
  />
  <template
    v-for="section in sections"
    :key="section.id"
  >
    <template v-if="section.type === 'hero'">
      <HeroShowcase
        data-home-section="hero"
        :slides="section.items"
        :assets="snapshot.assets"
        :locale="locale"
        :brand="brand"
        :artist-name="artistName"
      />
      <div
        class="next-section-edge"
        aria-hidden="true"
      >
        <span />
        <span />
        <span />
      </div>
    </template>

    <MusicSection
      v-else-if="section.type === 'music'"
      data-home-section="music"
      :section="section"
      :releases="snapshot.releases"
      :tracks="snapshot.tracks"
      :assets="snapshot.assets"
      :locale="locale"
      :active-track-id="player.current.value?.id ?? null"
      :player-status="player.status.value"
      @preview="player.toggle"
    />

    <VideoSection
      v-else-if="section.type === 'video'"
      data-home-section="video"
      :items="section.items"
      :assets="snapshot.assets"
      :locale="locale"
    />

    <EventSection
      v-else-if="section.type === 'event'"
      data-home-section="event"
      :items="section.items"
      :locale="locale"
    />

    <MomentSection
      v-else-if="section.type === 'moment'"
      data-home-section="moment"
      :items="section.items"
      :assets="snapshot.assets"
      :locale="locale"
    />

    <ArtistSection
      v-else-if="section.type === 'artist' && section.items[0]"
      data-home-section="artist"
      :artist="section.items[0]"
      :assets="snapshot.assets"
      :locale="locale"
    />
  </template>

  <SiteFooter
    :brand="brand"
    :canonical-url="snapshot.site.canonicalUrl"
    :social-links="snapshot.site.socialLinks"
    :locale="locale"
    :copyright-year="copyrightYear"
  />
  <LocaleNotice :raised="Boolean(player.current.value)" />
</template>

<style scoped>
.next-section-edge {
  display: grid;
  min-height: 3.5rem;
  grid-template-columns: 2fr 1fr 1fr;
  gap: 1px;
  padding: 1px;
  background: var(--color-border);
}

.next-section-edge span {
  background: var(--color-bg);
}
</style>
