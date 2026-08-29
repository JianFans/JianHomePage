<script setup lang="ts">
import type { Asset, Video } from '@yujian/schema'
import { ArrowUpRight, Film } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'

const props = defineProps<{
  items: Video[]
  assets: Asset[]
  locale: SupportedLocale
}>()

const assetById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const labels = computed(() => props.locale === 'en'
  ? { heading: 'Video', open: 'Open video' }
  : { heading: '影像', open: '打开影像' })
const cards = computed(() => props.items.flatMap((video) => {
  const poster = assetById.value.get(video.posterAssetId)
  const link = video.platformLinks.find((item) => {
    try {
      return new URL(item.url).protocol === 'https:'
    } catch {
      return false
    }
  })
  return poster && link ? [{ video, poster, link }] : []
}))

function durationLabel(seconds: number) {
  const minutes = Math.floor(seconds / 60)
  return `${minutes}:${String(seconds % 60).padStart(2, '0')}`
}
</script>

<template>
  <section
    id="video"
    class="video-section"
    aria-labelledby="video-section-title"
  >
    <div class="section-heading">
      <Film aria-hidden="true" />
      <h2 id="video-section-title">
        {{ labels.heading }}
      </h2>
    </div>

    <div class="video-grid">
      <a
        v-for="(card, index) in cards"
        :key="card.video.id"
        class="video-card"
        :class="{ 'video-card--featured': index === 0 }"
        data-testid="video-card"
        :href="card.link.url"
        target="_blank"
        rel="noopener noreferrer"
        :aria-label="`${labels.open}: ${resolveLocalized(card.video.title, locale)}`"
      >
        <img
          :src="card.poster.src"
          :alt="resolveLocalized(card.poster.alt, locale)"
          width="1600"
          height="900"
          loading="lazy"
        >
        <span class="video-scrim" />
        <span class="video-meta">
          <strong>{{ resolveLocalized(card.video.title, locale) }}</strong>
          <small>{{ durationLabel(card.video.durationSeconds) }}</small>
        </span>
        <ArrowUpRight
          class="video-open"
          aria-hidden="true"
        />
      </a>
    </div>
  </section>
</template>

<style scoped>
.video-section {
  padding: 0 max(1.25rem, calc((100vw - var(--content-max)) / 2)) clamp(5rem, 9vw, 8rem);
  background: var(--color-bg);
}

.section-heading {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  margin-bottom: 1.5rem;
}

.section-heading svg {
  width: 1.15rem;
  height: 1.15rem;
  color: var(--color-muted);
  stroke-width: 1.6;
}

.section-heading h2 {
  margin: 0;
  font-size: 0.95rem;
  font-weight: 580;
  letter-spacing: 0;
}

.video-grid {
  display: grid;
  aspect-ratio: 16 / 7.4;
  grid-template-columns: 1.62fr 1fr;
  grid-template-rows: repeat(2, minmax(0, 1fr));
  gap: 1px;
  background: var(--color-border);
}

.video-card {
  position: relative;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  background: var(--color-surface);
}

.video-card--featured {
  grid-row: 1 / 3;
}

.video-card img,
.video-scrim {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.video-card img {
  display: block;
  object-fit: cover;
  transition: transform 280ms var(--ease-standard);
}

.video-scrim {
  background: rgba(6, 8, 9, 0.38);
}

.video-meta {
  position: absolute;
  z-index: 1;
  right: 1rem;
  bottom: 1rem;
  left: 1rem;
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: 1rem;
}

.video-meta strong,
.video-meta small {
  letter-spacing: 0;
}

.video-meta strong {
  overflow: hidden;
  font-size: 0.95rem;
  font-weight: 560;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.video-meta small {
  color: var(--color-muted);
  font-size: 0.7rem;
}

.video-open {
  position: absolute;
  z-index: 1;
  top: 1rem;
  right: 1rem;
  width: 1.2rem;
  height: 1.2rem;
  stroke-width: 1.5;
}

.video-card:hover img,
.video-card:focus-visible img {
  transform: scale(1.018);
}

@media (max-width: 42rem) {
  .video-section {
    padding-bottom: 4.5rem;
  }

  .video-grid {
    aspect-ratio: auto;
    grid-template-columns: 1fr 1fr;
    grid-template-rows: auto;
  }

  .video-card,
  .video-card--featured {
    aspect-ratio: 16 / 10;
    grid-row: auto;
  }

  .video-card--featured {
    grid-column: 1 / -1;
    aspect-ratio: 4 / 3;
  }

  .video-meta {
    right: 0.75rem;
    bottom: 0.75rem;
    left: 0.75rem;
  }
}
</style>
