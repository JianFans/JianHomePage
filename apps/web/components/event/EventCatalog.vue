<script setup lang="ts">
import type { Event, YujianContentSnapshot } from '@yujian/schema'
import { ArrowUpRight, CalendarDays } from '@lucide/vue'
import { computed, ref } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import { EVENT_CATALOG_PATH, eventCatalogLabels, orderEvents } from '../../utils/event-catalog'
import { resolveHomepageSections } from '../../utils/sections'
import SiteFooter from '../site/SiteFooter.vue'
import SiteHeader from '../site/SiteHeader.vue'
import FallbackImage from '../ui/FallbackImage.vue'

const props = defineProps<{
  snapshot: YujianContentSnapshot
  locale: SupportedLocale
}>()
const emit = defineEmits<{ toggleLocale: [] }>()

const labels = computed(() => eventCatalogLabels[props.locale])
const brand = computed(() => resolveLocalized(props.snapshot.site.brand, props.locale))
const artistName = computed(() => resolveLocalized(props.snapshot.site.artistName, props.locale))
const copyrightYear = computed(() => new Date(props.snapshot.generatedAt).getUTCFullYear())
const homeSections = computed(() => resolveHomepageSections(props.snapshot, props.locale))
const assets = computed(() => new Map(props.snapshot.assets.map(asset => [asset.id, asset])))
const eventFilter = ref<'all' | 'upcoming' | 'past'>('all')
const allEvents = computed(() => orderEvents(props.snapshot).map(event => ({
  event,
  poster: event.posterAssetId ? assets.value.get(event.posterAssetId) : undefined,
  upcoming: event.status !== 'cancelled' && Date.parse(event.dateTime) > Date.parse(props.snapshot.generatedAt),
})))
const events = computed(() => allEvents.value.filter(({ upcoming }) => (
  eventFilter.value === 'all'
  || (eventFilter.value === 'upcoming' && upcoming)
  || (eventFilter.value === 'past' && !upcoming)
)))
const eventFilters = computed(() => [
  { id: 'all' as const, label: labels.value.all },
  { id: 'upcoming' as const, label: labels.value.upcomingFilter },
  { id: 'past' as const, label: labels.value.pastFilter },
])
const dateFormatter = computed(() => new Intl.DateTimeFormat(
  props.locale === 'en' ? 'en' : 'zh-CN',
  { year: 'numeric', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit', timeZone: 'UTC' },
))

function statusLabel(event: Event, upcoming: boolean) {
  if (event.status === 'cancelled') return labels.value.cancelled
  if (event.status === 'sold-out') return labels.value.soldOut
  return upcoming ? labels.value.upcoming : labels.value.ended
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
    :events-href="EVENT_CATALOG_PATH"
    events-current
    :show-video="homeSections.some(section => section.type === 'video')"
    @toggle-locale="emit('toggleLocale')"
  />
  <section
    class="event-catalog"
    aria-labelledby="event-catalog-title"
  >
    <header class="catalog-heading">
      <CalendarDays aria-hidden="true" />
      <div>
        <p>{{ artistName }}</p>
        <h1 id="event-catalog-title">
          {{ labels.heading }}
        </h1>
      </div>
    </header>
    <div
      v-if="allEvents.length"
      class="event-filters"
      role="group"
      :aria-label="labels.filter"
    >
      <button
        v-for="filter in eventFilters"
        :key="filter.id"
        type="button"
        :aria-label="filter.label"
        :aria-pressed="eventFilter === filter.id"
        @click="eventFilter = filter.id"
      >
        {{ filter.label }}
      </button>
    </div>
    <ol
      v-if="events.length"
      class="event-list"
    >
      <li
        v-for="{ event, poster, upcoming } in events"
        :id="event.id"
        :key="event.id"
        data-testid="event-card"
      >
        <a
          :href="event.detailUrl"
          :class="{ 'event-row--with-poster': poster }"
          target="_blank"
          rel="noopener noreferrer"
          :aria-label="`${labels.open}: ${resolveLocalized(event.title, locale)}`"
        >
          <FallbackImage
            v-if="poster"
            class="event-poster"
            :src="poster.src"
            :alt="resolveLocalized(poster.alt, locale)"
            width="720"
            height="480"
            loading="lazy"
            decoding="async"
          />
          <div class="event-details">
            <time :datetime="event.dateTime">{{ dateFormatter.format(new Date(event.dateTime)) }} UTC</time>
            <h2>{{ resolveLocalized(event.title, locale) }}</h2>
            <p>{{ resolveLocalized(event.city, locale) }} · {{ resolveLocalized(event.venue, locale) }}</p>
          </div>
          <span
            class="event-status"
            :class="{ 'event-status--upcoming': upcoming, 'event-status--cancelled': event.status === 'cancelled' }"
          >{{ statusLabel(event, upcoming) }}</span>
          <ArrowUpRight aria-hidden="true" />
        </a>
      </li>
    </ol>
    <p
      v-else
      class="catalog-empty"
      role="status"
    >
      {{ allEvents.length ? labels.filteredEmpty : labels.empty }}
    </p>
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
.event-catalog {
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
.event-filters { display: flex; flex-wrap: wrap; gap: 0.4rem; margin-bottom: 1.25rem; }
.event-filters button { min-height: 2.75rem; padding: 0.45rem 0.8rem; border: 1px solid var(--color-border); border-radius: var(--radius-tool); background: var(--color-surface); color: var(--color-muted); cursor: pointer; }
.event-filters button[aria-pressed="true"] { border-color: var(--color-accent); background: color-mix(in srgb, var(--color-accent) 14%, var(--color-surface)); color: var(--color-text); }
.event-filters button:hover { border-color: var(--color-accent); color: var(--color-text); }
.event-list { padding: 0; margin: 0; border-top: 1px solid var(--color-border); list-style: none; }
.event-list li { border-bottom: 1px solid var(--color-border); }
.event-list a { display: grid; min-height: 7rem; grid-template-columns: minmax(0, 1fr) auto 2.75rem; align-items: center; gap: 1.25rem; color: inherit; text-decoration: none; }
.event-list a.event-row--with-poster { grid-template-columns: 7rem minmax(0, 1fr) auto 2.75rem; }
.event-poster { display: block; width: 7rem; aspect-ratio: 3 / 2; object-fit: cover; background: var(--color-surface); }
.event-details { min-width: 0; }
.event-details time { color: var(--color-warm); font-size: 0.85rem; }
.event-details h2 { margin: 0.3rem 0; font-size: 1.1rem; font-weight: 560; }
.event-details p { margin: 0; color: var(--color-muted); font-size: 0.82rem; }
.event-status { padding: 0.35rem 0.55rem; border: 1px solid var(--color-border); color: var(--color-muted); font-size: 0.72rem; white-space: nowrap; }
.event-status--upcoming { border-color: color-mix(in srgb, var(--color-accent) 48%, var(--color-border)); color: var(--color-accent); }
.event-status--cancelled { color: var(--color-warm); }
.event-list a > svg { width: 1.1rem; height: 1.1rem; justify-self: end; color: var(--color-muted); }
.event-list a:hover h2, .event-list a:focus-visible h2 { color: var(--color-focus); }
.event-list a:hover > svg, .event-list a:focus-visible > svg { color: var(--color-text); }

@media (max-width: 48rem) {
  .event-catalog { padding: calc(var(--header-height) + 2rem) 1rem 3rem; }
  .catalog-heading { margin-bottom: 1.75rem; }
  .catalog-heading h1 { font-size: 2rem; }
  .event-list a,
  .event-list a.event-row--with-poster { min-height: 6rem; grid-template-columns: minmax(0, 1fr) auto 2rem; gap: 0.75rem; }
  .event-poster { display: none; }
  .event-status { grid-column: 2; grid-row: 1; }
  .event-list a > svg { grid-column: 3; grid-row: 1; }
  .event-details { grid-column: 1; grid-row: 1; }
}

@media (prefers-reduced-motion: reduce) {
  .event-list a, .event-list a > svg { transition: none; }
}
</style>
