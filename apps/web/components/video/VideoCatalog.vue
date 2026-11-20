<script setup lang="ts">
import type { Asset, Video, YujianContentSnapshot } from '@yujian/schema'
import { ArrowUpRight, Film } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import { VIDEO_CATALOG_PATH, videoCatalogLabels } from '../../utils/video-catalog'
import { resolveHomepageSections } from '../../utils/sections'
import SiteFooter from '../site/SiteFooter.vue'
import SiteHeader from '../site/SiteHeader.vue'
import FallbackImage from '../ui/FallbackImage.vue'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
}>()
const emit = defineEmits<{ toggleLocale: [] }>()

const labels = computed(() => videoCatalogLabels[props.locale])
const brand = computed(() => resolveLocalized(props.snapshot.site.brand, props.locale))
const artistName = computed(() => resolveLocalized(props.snapshot.site.artistName, props.locale))
const copyrightYear = computed(() => new Date(props.snapshot.generatedAt).getUTCFullYear())
const homeSections = computed(() => resolveHomepageSections(props.snapshot, props.locale))
const assetById = computed(() => new Map(props.snapshot.assets.map(asset => [asset.id, asset])))

const videos = computed(() => props.snapshot.videos
  .map((video) => {
    const poster = assetById.value.get(video.posterAssetId)
    const links = video.platformLinks.filter(link => isHttps(link.url))
    return poster && links.length ? { video, poster, links } : null
  })
  .filter((item): item is { video: Video, poster: Asset, links: Video['platformLinks'] } => Boolean(item))
  .sort((left, right) => left.video.featured === right.video.featured
    ? 0
    : left.video.featured ? -1 : 1))

function isHttps(value: string) {
  try {
    return new URL(value).protocol === 'https:'
  } catch {
    return false
  }
}

function durationLabel(seconds: number) {
  return `${Math.floor(seconds / 60)}:${String(seconds % 60).padStart(2, '0')}`
}
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    home-href="/"
    music-href="/music/"
    :video-href="VIDEO_CATALOG_PATH"
    video-current
    :show-events="homeSections.some(section => section.type === 'event')"
    @toggle-locale="emit('toggleLocale')"
  />
  <section
    class="video-catalog"
    aria-labelledby="video-catalog-title"
  >
    <header class="catalog-heading">
      <Film aria-hidden="true" />
      <div>
        <p>{{ artistName }}</p>
        <h1 id="video-catalog-title">
          {{ labels.heading }}
        </h1>
      </div>
    </header>
    <p
      v-if="!videos.length"
      class="catalog-empty"
    >
      {{ labels.empty }}
    </p>
    <div
      v-else
      class="video-grid"
    >
      <article
        v-for="({ video, poster, links }, index) in videos"
        :id="video.id"
        :key="video.id"
        class="video-card"
        :class="{ 'video-card--featured': index === 0 }"
        :aria-labelledby="`${video.id}-title`"
      >
        <FallbackImage
          class="video-poster"
          :src="poster.src"
          :alt="resolveLocalized(poster.alt, locale)"
          width="1600"
          height="900"
          loading="lazy"
          decoding="async"
        />
        <span class="video-scrim" />
        <div class="video-meta">
          <div>
            <h2 :id="`${video.id}-title`">
              {{ resolveLocalized(video.title, locale) }}
            </h2>
            <span>{{ durationLabel(video.durationSeconds) }}</span>
          </div>
          <PlatformLinks
            :links="links"
            :locale="locale"
            menu-align="end"
          />
        </div>
        <a
          class="video-card__open"
          :href="links[0]!.url"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="`${labels.open}: ${resolveLocalized(video.title, locale)}`"
        ><ArrowUpRight aria-hidden="true" /></a>
      </article>
    </div>
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
.video-catalog {
  width: min(100%, var(--content-max));
  min-height: 65vh;
  padding: calc(var(--header-height) + 3.5rem) 1.25rem 4rem;
  margin-inline: auto;
}

.catalog-heading { display: flex; align-items: center; gap: 0.8rem; margin-bottom: 2.5rem; }
.catalog-heading > svg { width: 1.5rem; height: 1.5rem; color: var(--color-accent); }
.catalog-heading p { margin: 0 0 0.3rem; color: var(--color-accent); }
.catalog-heading h1 { margin: 0; font-size: 2.75rem; font-weight: 620; line-height: 1.15; }
.catalog-empty { padding-block: 2rem; border-top: 1px solid var(--color-border); color: var(--color-muted); }

.video-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1px; background: var(--color-border); }
.video-card { position: relative; min-width: 0; aspect-ratio: 16 / 9; overflow: hidden; background: var(--color-surface); }
.video-card--featured { grid-column: 1 / -1; aspect-ratio: 16 / 7; }
.video-poster, .video-scrim { position: absolute; inset: 0; width: 100%; height: 100%; }
.video-poster { display: block; object-fit: cover; transition: transform var(--motion-slow) var(--motion-ease-out); }
.video-scrim { background: linear-gradient(180deg, rgba(6, 8, 9, 0.12), rgba(6, 8, 9, 0.72)); pointer-events: none; }
.video-meta { position: absolute; z-index: 1; right: 1rem; bottom: 1rem; left: 1rem; display: flex; align-items: end; justify-content: space-between; gap: 1rem; }
.video-meta h2 { margin: 0; font-size: 1.1rem; font-weight: 560; line-height: 1.35; }
.video-meta span { display: inline-block; margin-top: 0.3rem; color: var(--color-muted); font-size: 0.75rem; }
.video-meta :deep(.platform-links) { flex: 0 0 auto; }
.video-card__open { position: absolute; z-index: 2; top: 0.75rem; right: 0.75rem; display: grid; width: 2.75rem; height: 2.75rem; place-items: center; border: 1px solid var(--color-border); background: color-mix(in srgb, var(--color-bg) 72%, transparent); }
.video-card__open svg { width: 1.15rem; height: 1.15rem; }
.video-card:hover .video-poster, .video-card:focus-within .video-poster { transform: scale(1.018); }
.catalog-empty, .video-card, .video-card__open { transition: background-color var(--motion-fast) var(--motion-ease-out); }

@media (max-width: 48rem) {
  .video-catalog { padding: calc(var(--header-height) + 2rem) 1rem 3rem; }
  .catalog-heading { margin-bottom: 1.75rem; }
  .catalog-heading h1 { font-size: 2rem; }
}
@media (max-width: 36rem) {
  .video-grid { grid-template-columns: 1fr; }
  .video-card--featured { grid-column: auto; aspect-ratio: 16 / 9; }
  .video-meta { right: 0.75rem; bottom: 0.75rem; left: 0.75rem; }
  .video-meta h2 { font-size: 0.95rem; }
}
@media (prefers-reduced-motion: reduce) {
  .video-poster, .catalog-empty, .video-card, .video-card__open { transition: none; }
}
</style>
