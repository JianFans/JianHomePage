<script setup lang="ts">
import type { Moment, YujianContentSnapshot } from '@yujian/schema'
import { Images } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import { MOMENT_CATALOG_PATH, internalContentUrl, momentCatalogLabels } from '../../utils/moments-catalog'
import { resolveHomepageSections } from '../../utils/sections'
import SiteFooter from '../site/SiteFooter.vue'
import SiteHeader from '../site/SiteHeader.vue'
import FallbackImage from '../ui/FallbackImage.vue'

const props = defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
}>()
const emit = defineEmits<{ toggleLocale: [] }>()

const labels = computed(() => momentCatalogLabels[props.locale])
const brand = computed(() => resolveLocalized(props.snapshot.site.brand, props.locale))
const artistName = computed(() => resolveLocalized(props.snapshot.site.artistName, props.locale))
const copyrightYear = computed(() => new Date(props.snapshot.generatedAt).getUTCFullYear())
const homeSections = computed(() => resolveHomepageSections(props.snapshot, props.locale))
const assets = computed(() => new Map(props.snapshot.assets.map(asset => [asset.id, asset])))
const moments = computed(() => props.snapshot.moments.flatMap((moment) => {
  const asset = assets.value.get(moment.assetId)
  if (!asset) return []
  const target = moment.target
  const href = !target
    ? null
    : target.kind === 'external'
      ? target.link.url
      : internalContentUrl(props.snapshot, target.contentId)
  return [{ moment, asset, href, external: target?.kind === 'external' }]
}))

function momentLabel(moment: Moment) {
  return moment.caption ? resolveLocalized(moment.caption, props.locale) : labels.value.open
}
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    home-href="/"
    music-href="/music/"
    video-href="/video/"
    events-href="/events/"
    :moments-href="MOMENT_CATALOG_PATH"
    moments-current
    :show-video="homeSections.some(section => section.type === 'video')"
    @toggle-locale="emit('toggleLocale')"
  />
  <main
    class="moment-catalog"
    aria-labelledby="moment-catalog-title"
  >
    <header class="catalog-heading">
      <Images aria-hidden="true" />
      <div>
        <p>{{ artistName }}</p>
        <h1 id="moment-catalog-title">
          {{ labels.heading }}
        </h1>
      </div>
    </header>
    <p
      v-if="!moments.length"
      class="catalog-empty"
    >
      {{ labels.empty }}
    </p>
    <ol
      v-else
      class="moment-grid"
    >
      <li
        v-for="{ moment, asset, href, external } in moments"
        :id="moment.id"
        :key="moment.id"
        data-testid="moment-card"
      >
        <figure :style="{ aspectRatio: asset.width && asset.height ? `${asset.width} / ${asset.height}` : '4 / 5' }">
          <component
            :is="href ? 'a' : 'div'"
            class="moment-content"
            :href="href ?? undefined"
            :target="external ? '_blank' : undefined"
            :rel="external ? 'noopener noreferrer' : undefined"
            :aria-label="href ? momentLabel(moment) : undefined"
          >
            <FallbackImage
              :src="asset.src"
              :alt="resolveLocalized(asset.alt, locale)"
              :width="asset.width"
              :height="asset.height"
              loading="lazy"
              decoding="async"
            />
            <figcaption v-if="moment.caption">
              {{ resolveLocalized(moment.caption, locale) }}
            </figcaption>
          </component>
        </figure>
      </li>
    </ol>
  </main>
  <SiteFooter
    :brand="brand"
    :canonical-url="snapshot.site.canonicalUrl"
    :social-links="snapshot.site.socialLinks"
    :locale="locale"
    :copyright-year="copyrightYear"
  />
</template>

<style scoped>
.moment-catalog {
  width: min(100%, var(--content-max));
  min-height: 65vh;
  padding: calc(var(--header-height) + 3.5rem) 1.25rem 4rem;
  margin-inline: auto;
}

.catalog-heading { display: flex; align-items: center; gap: 0.8rem; margin-bottom: 2rem; }
.catalog-heading > svg { width: 1.5rem; height: 1.5rem; color: var(--color-accent); }
.catalog-heading p { margin: 0 0 0.3rem; color: var(--color-accent); }
.catalog-heading h1 { margin: 0; font-size: 2.75rem; font-weight: 620; line-height: 1.15; }
.catalog-empty { padding-block: 2rem; border-top: 1px solid var(--color-border); color: var(--color-muted); }
.moment-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 1px; padding: 0; margin: 0; background: var(--color-border); list-style: none; }
.moment-grid li { min-width: 0; background: var(--color-bg); }
.moment-grid figure { min-width: 0; margin: 0; overflow: hidden; background: var(--color-surface); }
.moment-content { position: relative; display: block; width: 100%; height: 100%; color: inherit; }
.moment-content img { display: block; width: 100%; height: 100%; object-fit: cover; transition: transform var(--motion-slow) var(--motion-ease-out); }
.moment-content:hover img, .moment-content:focus-visible img { transform: scale(1.015); }
.moment-content figcaption { position: absolute; inset: auto 0 0; padding: 2rem 1rem 0.9rem; background: linear-gradient(transparent, rgb(0 0 0 / 0.78)); color: #fff; font-size: 0.85rem; line-height: 1.5; }

@media (max-width: 52rem) {
  .moment-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 34rem) {
  .moment-catalog { padding: calc(var(--header-height) + 2rem) 1rem 3rem; }
  .catalog-heading h1 { font-size: 2rem; }
  .moment-grid { grid-template-columns: minmax(0, 1fr); }
}

@media (prefers-reduced-motion: reduce) {
  .moment-content img { transition: none; }
}
</style>
