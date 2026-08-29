<script setup lang="ts">
import type {
  Asset,
  MusicSection,
  Release,
  Track,
} from '@yujian/schema'
import { ArrowUpRight, Headphones, LoaderCircle, Pause, Play } from '@lucide/vue'
import { computed } from 'vue'
import type {
  AudioPlayerStatus,
  AudioPlayerTrack,
} from '../../composables/useAudioPlayer'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import FallbackImage from '../ui/FallbackImage.vue'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  section: MusicSection
  releases: Release[]
  tracks: Track[]
  assets: Asset[]
  locale: SupportedLocale
  activeTrackId: string | null
  playerStatus: AudioPlayerStatus
}>()

const emit = defineEmits<{
  preview: [track: AudioPlayerTrack, queue: AudioPlayerTrack[]]
}>()

const releaseById = computed(() => new Map(props.releases.map(release => [release.id, release])))
const trackById = computed(() => new Map(props.tracks.map(track => [track.id, track])))
const assetById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const labels = computed(() => props.locale === 'en'
  ? { heading: 'Music', preview: 'Preview', all: 'View all music' }
  : { heading: '音乐', preview: '试听', all: '查看全部音乐' })

const cards = computed(() => props.section.itemIds
  .slice(0, props.section.limit)
  .map((id) => {
    const release = releaseById.value.get(id)
    if (!release) {
      return null
    }
    const cover = assetById.value.get(release.coverAssetId)
    const track = release.trackIds
      .map(trackId => trackById.value.get(trackId))
      .find(item => item?.previewAssetId && assetById.value.get(item.previewAssetId)?.kind === 'audio')
    const preview = track?.previewAssetId
      ? assetById.value.get(track.previewAssetId)
      : undefined
    const playable: AudioPlayerTrack | null = track && preview
      ? {
          id: track.id,
          title: track.title,
          previewSrc: preview.src,
          coverSrc: cover?.src,
          platformLinks: track.platformLinks.length ? track.platformLinks : release.platformLinks,
        }
      : null
    return { release, cover, playable }
  })
  .filter((card): card is NonNullable<typeof card> => Boolean(card)))
const playableQueue = computed(() => cards.value.flatMap(card => (
  card.playable ? [card.playable] : []
)))
const moreHref = computed(() => {
  try {
    return props.section.moreLink && new URL(props.section.moreLink.url).protocol === 'https:'
      ? props.section.moreLink.url
      : null
  } catch {
    return null
  }
})

function previewLabel(track: AudioPlayerTrack) {
  const title = resolveLocalized(track.title, props.locale)
  return props.locale === 'en'
    ? `${labels.value.preview} ${title}`
    : `${labels.value.preview}${title}`
}

function isActive(track: AudioPlayerTrack) {
  return props.activeTrackId === track.id
}

function preview(track: AudioPlayerTrack) {
  emit('preview', track, playableQueue.value)
}
</script>

<template>
  <section
    id="music"
    class="music-section"
    :aria-labelledby="`${section.id}-title`"
  >
    <div class="section-heading">
      <Headphones aria-hidden="true" />
      <h2 :id="`${section.id}-title`">
        {{ labels.heading }}
      </h2>
      <a
        v-if="moreHref"
        class="section-more"
        data-testid="music-more"
        :href="moreHref"
        target="_blank"
        rel="noopener noreferrer"
        :aria-label="labels.all"
        :title="labels.all"
      >
        <ArrowUpRight aria-hidden="true" />
      </a>
    </div>

    <div class="music-grid">
      <article
        v-for="card in cards"
        :id="card.release.id"
        :key="card.release.id"
        class="music-card"
        :class="{ 'music-card--featured': card.release.featured }"
        data-testid="music-card"
      >
        <div class="music-cover">
          <button
            v-if="card.playable"
            type="button"
            class="music-cover-action"
            data-testid="preview-trigger"
            :aria-label="previewLabel(card.playable)"
            :aria-pressed="isActive(card.playable) && playerStatus === 'playing'"
            @click="preview(card.playable)"
          >
            <FallbackImage
              v-if="card.cover"
              :src="card.cover.src"
              :alt="resolveLocalized(card.cover.alt, locale)"
              width="720"
              height="720"
              loading="lazy"
            />
            <span class="music-cover-state">
              <LoaderCircle
                v-if="isActive(card.playable) && playerStatus === 'loading'"
                aria-hidden="true"
              />
              <Pause
                v-else-if="isActive(card.playable) && playerStatus === 'playing'"
                aria-hidden="true"
              />
              <Play
                v-else
                aria-hidden="true"
              />
            </span>
          </button>
          <FallbackImage
            v-else-if="card.cover"
            :src="card.cover.src"
            :alt="resolveLocalized(card.cover.alt, locale)"
            width="720"
            height="720"
            loading="lazy"
          />
        </div>

        <div class="music-meta">
          <h3>{{ resolveLocalized(card.release.title, locale) }}</h3>
          <time :datetime="card.release.releaseDate">
            {{ card.release.releaseDate.slice(0, 4) }}
          </time>
        </div>

        <PlatformLinks
          :links="card.release.platformLinks"
          :locale="locale"
        />
      </article>
    </div>
  </section>
</template>

<style scoped>
.music-section {
  padding: clamp(4.5rem, 9vw, 8rem) max(1.25rem, calc((100vw - var(--content-max)) / 2));
  background: var(--color-bg);
}

.section-heading {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  margin-bottom: 1.5rem;
  color: var(--color-muted);
}

.section-heading svg {
  width: 1.15rem;
  height: 1.15rem;
  stroke-width: 1.6;
}

.section-heading h2 {
  margin: 0;
  color: var(--color-text);
  font-size: 0.95rem;
  font-weight: 580;
  letter-spacing: 0;
}

.section-more {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  margin-left: auto;
  place-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  color: var(--color-muted);
}

.section-more:hover,
.section-more:focus-visible {
  border-color: var(--color-accent);
  color: var(--color-text);
}

.section-more svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.6;
}

.music-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  border: 1px solid var(--color-border);
  background: var(--color-border);
}

.music-card {
  display: grid;
  min-width: 0;
  grid-template-columns: 6.5rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem;
  background: var(--color-surface);
}

.music-card--featured:first-child {
  grid-column: 1 / -1;
  grid-template-columns: 9rem minmax(0, 1fr) auto;
  background: var(--color-surface-raised);
}

.music-cover,
.music-cover img,
.music-cover-action {
  width: 100%;
  aspect-ratio: 1;
}

.music-cover {
  position: relative;
  overflow: hidden;
  background: var(--color-bg);
}

.music-cover img {
  display: block;
  object-fit: cover;
}

.music-cover-action {
  position: relative;
  display: block;
  padding: 0;
  overflow: hidden;
  border: 0;
  border-radius: 0;
  background: transparent;
  color: var(--color-text);
  cursor: pointer;
}

.music-cover-state {
  position: absolute;
  inset: 0;
  display: grid;
  place-items: center;
  background: rgba(6, 8, 9, 0.52);
  opacity: 0;
  transition: opacity 160ms var(--ease-standard);
}

.music-cover-state svg {
  width: 1.6rem;
  height: 1.6rem;
  stroke-width: 1.6;
}

.music-cover-action:hover .music-cover-state,
.music-cover-action:focus-visible .music-cover-state,
.music-cover-action[aria-pressed="true"] .music-cover-state {
  opacity: 1;
}

.music-meta {
  min-width: 0;
}

.music-meta h3,
.music-meta time {
  display: block;
  margin: 0;
  letter-spacing: 0;
}

.music-meta h3 {
  overflow: hidden;
  font-size: clamp(1rem, 1.45rem, 1.45rem);
  font-weight: 540;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.music-meta time {
  margin-top: 0.35rem;
  color: var(--color-muted);
  font-size: 0.72rem;
}

@media (max-width: 54rem) {
  .music-grid {
    grid-template-columns: 1fr;
  }

  .music-card--featured:first-child {
    grid-column: auto;
    grid-template-columns: 7.5rem minmax(0, 1fr) auto;
  }
}

@media (max-width: 42rem) {
  .music-section {
    padding-block: 4.5rem;
  }

  .music-card,
  .music-card--featured:first-child {
    grid-template-columns: 5.5rem minmax(0, 1fr) auto;
    gap: 0.75rem;
    padding: 0.55rem;
  }

  .music-meta h3 {
    font-size: 1rem;
  }

  .music-cover-state {
    opacity: 1;
  }
}
</style>
