<script setup lang="ts">
import type { YujianContentSnapshot } from '@yujian/schema'
import { computed } from 'vue'
import type { AudioPlayerController } from '../../utils/audio-player'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import { resolveHomepageSections } from '../../utils/sections'
import SiteFooter from '../site/SiteFooter.vue'
import SiteHeader from '../site/SiteHeader.vue'
import ArtistSection from './ArtistSection.vue'
import EventSection from './EventSection.vue'
import HeroShowcase from './HeroShowcase.vue'
import MomentSection from './MomentSection.vue'
import MusicSection from './MusicSection.vue'
import VideoSection from './VideoSection.vue'

const props = withDefaults(defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
  player: AudioPlayerController
  autoplayEnabled?: boolean
}>(), { autoplayEnabled: true })

const emit = defineEmits<{
  toggleLocale: []
}>()

// Use the snapshot clock for deterministic public builds and draft previews alike.
const sections = computed(() => resolveHomepageSections(props.snapshot, props.locale))
const artistVisible = computed(() => sections.value.some(section => section.type === 'artist' && Boolean(section.items[0]?.platformLinks.length)))
const brand = computed(() => resolveLocalized(props.snapshot.site.brand, props.locale))
const artistName = computed(() => resolveLocalized(props.snapshot.site.artistName, props.locale))
const copyrightYear = computed(() => new Date(props.snapshot.generatedAt).getUTCFullYear())
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    @toggle-locale="emit('toggleLocale')"
  />
  <template
    v-for="(section, index) in sections"
    :key="index"
  >
    <template v-if="section.type === 'hero'">
      <HeroShowcase
        data-home-section="hero"
        :slides="section.items"
        :assets="snapshot.assets"
        :locale="locale"
        :brand="brand"
        :artist-name="artistName"
        :autoplay-enabled="autoplayEnabled"
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
    :show-social-links="!artistVisible"
    :locale="locale"
    :copyright-year="copyrightYear"
  />
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
