<script setup lang="ts">
import type { Event } from '@yujian/schema'
import { ArrowUpRight, CalendarDays } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'

const props = defineProps<{
  items: Event[]
  locale: SupportedLocale
}>()

const labels = computed(() => props.locale === 'en'
  ? { heading: 'Live', open: 'Open event details' }
  : { heading: '现场', open: '打开现场详情' })
const dateFormatter = computed(() => new Intl.DateTimeFormat(
  props.locale === 'en' ? 'en' : 'zh-CN',
  { month: 'short', day: '2-digit', timeZone: 'UTC' },
))
</script>

<template>
  <section
    id="event"
    class="event-section"
    aria-labelledby="event-section-title"
  >
    <div class="event-inner">
      <div class="section-heading">
        <CalendarDays aria-hidden="true" />
        <h2 id="event-section-title">
          {{ labels.heading }}
        </h2>
      </div>

      <ol class="event-list">
        <li
          v-for="event in items"
          :key="event.id"
          data-testid="event-item"
        >
          <a
            :href="event.detailUrl"
            target="_blank"
            rel="noopener noreferrer"
            :aria-label="`${labels.open}: ${resolveLocalized(event.title, locale)}`"
          >
            <time :datetime="event.dateTime">
              {{ dateFormatter.format(new Date(event.dateTime)) }}
            </time>
            <strong>{{ resolveLocalized(event.title, locale) }}</strong>
            <span>
              {{ resolveLocalized(event.city, locale) }} · {{ resolveLocalized(event.venue, locale) }}
            </span>
            <ArrowUpRight aria-hidden="true" />
          </a>
        </li>
      </ol>
    </div>
  </section>
</template>

<style scoped>
.event-section {
  border-block: 1px solid var(--color-border);
  background: var(--color-surface);
}

.event-inner {
  padding: clamp(4.5rem, 8vw, 7rem) max(1.25rem, calc((100vw - var(--content-max)) / 2));
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

.event-list {
  margin: 0;
  padding: 0;
  border-top: 1px solid var(--color-border);
  list-style: none;
}

.event-list li {
  border-bottom: 1px solid var(--color-border);
}

.event-list a {
  display: grid;
  min-height: 5.5rem;
  grid-template-columns: 7rem minmax(10rem, 1fr) minmax(12rem, 1fr) 2.75rem;
  align-items: center;
  gap: 1rem;
  color: inherit;
  text-decoration: none;
}

.event-list time,
.event-list strong,
.event-list span {
  letter-spacing: 0;
}

.event-list time {
  color: var(--color-warm);
  font-size: 1.2rem;
  font-weight: 540;
}

.event-list strong {
  font-size: 1rem;
  font-weight: 550;
}

.event-list span {
  color: var(--color-muted);
  font-size: 0.8rem;
}

.event-list svg {
  width: 1.1rem;
  height: 1.1rem;
  justify-self: end;
  stroke-width: 1.5;
}

.event-list a:hover strong,
.event-list a:focus-visible strong {
  color: var(--color-focus);
}

@media (max-width: 42rem) {
  .event-list a {
    min-height: 6.5rem;
    grid-template-columns: 4.25rem minmax(0, 1fr) 2rem;
    grid-template-rows: auto auto;
    gap: 0.25rem 0.75rem;
  }

  .event-list time {
    grid-row: 1 / 3;
    font-size: 0.9rem;
  }

  .event-list span {
    grid-column: 2;
  }

  .event-list svg {
    grid-column: 3;
    grid-row: 1 / 3;
  }
}
</style>
