<script setup lang="ts">
import type { YujianContentSnapshot } from '@yujian/schema'
import { Pause, Play } from '@lucide/vue'
import { computed } from 'vue'
import type { AudioPlayerController, AudioPlayerTrack } from '../../utils/audio-player'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import { MUSIC_CATALOG_PATH, musicCatalogLabels } from '../../utils/music-catalog'
import { resolveHomepageSections } from '../../utils/sections'
import SiteFooter from '../site/SiteFooter.vue'
import SiteHeader from '../site/SiteHeader.vue'
import ActivityIndicator from '../ui/ActivityIndicator.vue'
import FallbackImage from '../ui/FallbackImage.vue'
import IconButton from '../ui/IconButton.vue'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
  player: AudioPlayerController
}>()

const emit = defineEmits<{ toggleLocale: [] }>()
const labels = computed(() => musicCatalogLabels[props.locale])
const brand = computed(() => resolveLocalized(props.snapshot.site.brand, props.locale))
const artistName = computed(() => resolveLocalized(props.snapshot.site.artistName, props.locale))
const copyrightYear = computed(() => new Date(props.snapshot.generatedAt).getUTCFullYear())
const homeSections = computed(() => resolveHomepageSections(props.snapshot, props.locale))

const releases = computed(() => {
  const assets = new Map(props.snapshot.assets.map(asset => [asset.id, asset]))
  const tracks = new Map(props.snapshot.tracks.map(track => [track.id, track]))
  const ordered = [...props.snapshot.releases].sort((left, right) => (
    left.releaseDate < right.releaseDate ? 1 : left.releaseDate > right.releaseDate ? -1 : 0
  ))

  return ordered.map((release) => {
    const cover = assets.get(release.coverAssetId)
    const rows = release.trackIds.flatMap((id) => {
      const track = tracks.get(id)
      if (!track) return []
      const preview = track.previewAssetId ? assets.get(track.previewAssetId) : undefined
      const playable: AudioPlayerTrack | null = preview?.kind === 'audio'
        ? {
            id: track.id,
            title: track.title,
            previewSrc: preview.src,
            coverSrc: cover?.src,
            platformLinks: track.platformLinks.length ? track.platformLinks : release.platformLinks,
          }
        : null
      return [{ track, playable }]
    })
    return { release, cover, rows }
  })
})

const queue = computed(() => releases.value.flatMap(({ rows }) => rows.flatMap(({ playable }) => playable ?? [])))

function isPlaying(id: string) {
  return props.player.current.value?.id === id && props.player.status.value === 'playing'
}

function isLoading(id: string) {
  return props.player.current.value?.id === id && props.player.status.value === 'loading'
}
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    home-href="/"
    :music-href="MUSIC_CATALOG_PATH"
    music-current
    :show-video="homeSections.some(section => section.type === 'video')"
    :show-events="homeSections.some(section => section.type === 'event')"
    @toggle-locale="emit('toggleLocale')"
  />
  <section
    class="music-catalog"
    aria-labelledby="music-catalog-title"
  >
    <header class="catalog-heading">
      <p>{{ artistName }}</p>
      <h1 id="music-catalog-title">
        {{ labels.heading }}
      </h1>
    </header>
    <p
      v-if="!releases.length"
      class="catalog-empty"
    >
      {{ labels.empty }}
    </p>
    <article
      v-for="{ release, cover, rows } in releases"
      :id="release.id"
      :key="release.id"
      class="catalog-release"
      :aria-labelledby="`${release.id}-title`"
    >
      <FallbackImage
        v-if="cover"
        class="release-cover"
        :src="cover.src"
        :alt="resolveLocalized(cover.alt, locale)"
        width="720"
        height="720"
        loading="lazy"
        decoding="async"
      />
      <div class="release-info">
        <h2 :id="`${release.id}-title`">
          {{ resolveLocalized(release.title, locale) }}
        </h2>
        <time :datetime="release.releaseDate">{{ release.releaseDate }}</time>
        <PlatformLinks
          :links="release.platformLinks"
          :locale="locale"
          menu-align="start"
        />
      </div>
      <ul
        v-if="rows.length"
        class="release-tracks"
        role="list"
      >
        <li
          v-for="{ track, playable } in rows"
          :id="track.id"
          :key="track.id"
          class="catalog-track"
        >
          <IconButton
            v-if="playable"
            :label="labels.preview + resolveLocalized(track.title, locale)"
            :pressed="isPlaying(track.id)"
            :aria-busy="isLoading(track.id) || undefined"
            @click="player.toggle(playable, queue)"
          >
            <ActivityIndicator
              v-if="isLoading(track.id)"
              aria-hidden="true"
            />
            <Pause
              v-else-if="isPlaying(track.id)"
              aria-hidden="true"
            />
            <Play
              v-else
              aria-hidden="true"
            />
          </IconButton>
          <span class="track-title">{{ resolveLocalized(track.title, locale) }}</span>
          <PlatformLinks
            :links="track.platformLinks"
            :locale="locale"
          />
        </li>
      </ul>
    </article>
  </section>
  <SiteFooter
    :brand="brand"
    :canonical-url="snapshot.site.canonicalUrl"
    :social-links="snapshot.site.socialLinks"
    :locale="locale"
    :copyright-year="copyrightYear"
  />
</template>

<style scoped>
.music-catalog {
  width: min(100%, var(--content-max));
  min-height: 65vh;
  padding: calc(var(--header-height) + 3.5rem) 1.25rem 4rem;
  margin-inline: auto;
}

.catalog-heading { margin-bottom: 2.5rem; }
.catalog-heading p { margin: 0 0 0.3rem; color: var(--color-accent); }
.catalog-heading h1 { margin: 0; font-size: 2.75rem; font-weight: 620; line-height: 1.15; }
.catalog-empty { padding-block: 2rem; border-top: 1px solid var(--color-border); color: var(--color-muted); }

.catalog-release {
  display: grid;
  grid-template-columns: 8rem minmax(9rem, 1fr) minmax(0, 2.2fr);
  align-items: start;
  gap: 1.25rem;
  padding-block: 1.5rem;
  border-top: 1px solid var(--color-border);
  scroll-margin-top: calc(var(--header-height) + 1rem);
}

.release-cover { display: block; width: 100%; height: auto; aspect-ratio: 1; object-fit: cover; }
.release-info { grid-column: 2; min-width: 0; }
.release-info h2 { margin: 0; font-size: 1.25rem; font-weight: 580; line-height: 1.25; overflow-wrap: anywhere; }
.release-info time { display: block; margin: 0.3rem 0 0.65rem; color: var(--color-warm); font-size: 0.8rem; }
.release-tracks { grid-column: 3; min-width: 0; padding: 0; margin: 0; list-style: none; }

.catalog-track {
  display: grid;
  min-height: 3.5rem;
  grid-template-columns: 2.75rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 0.65rem;
  padding: 0.35rem 0.5rem;
  border-bottom: 1px solid var(--color-border);
  scroll-margin-top: calc(var(--header-height) + 0.75rem);
  transition: background-color var(--motion-fast) var(--motion-ease-out);
}

.catalog-track:hover,
.catalog-track:focus-within { background: var(--color-surface); }
.track-title { grid-column: 2; overflow-wrap: anywhere; }

@media (max-width: 48rem) {
  .music-catalog { padding: calc(var(--header-height) + 2rem) 1rem 3rem; }
  .catalog-heading { margin-bottom: 1.75rem; }
  .catalog-heading h1 { font-size: 2rem; }
  .catalog-release { grid-template-columns: 4.75rem minmax(0, 1fr); gap: 0.75rem; }
  .release-tracks { grid-column: 1 / -1; }
}
</style>
