<script setup lang="ts">
import type { Asset, Moment } from '@yujian/schema'
import { Images } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import FallbackImage from '../ui/FallbackImage.vue'

const props = defineProps<{
  items: Moment[]
  assets: Asset[]
  locale: SupportedLocale
}>()

const assetById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const visibleItems = computed(() => props.items.flatMap((moment) => {
  const asset = assetById.value.get(moment.assetId)
  return asset ? [{ moment, asset }] : []
}))
const heading = computed(() => props.locale === 'en' ? 'Moments' : '片段')
</script>

<template>
  <section
    id="moment"
    class="moment-section"
    :aria-label="heading"
  >
    <div
      class="moment-heading"
      aria-hidden="true"
    >
      <Images />
    </div>
    <div class="moment-grid">
      <figure
        v-for="item in visibleItems"
        :key="item.moment.id"
        data-testid="moment-item"
      >
        <FallbackImage
          :src="item.asset.src"
          :alt="resolveLocalized(item.asset.alt, locale)"
          :width="item.asset.width"
          :height="item.asset.height"
          loading="lazy"
        />
        <figcaption v-if="item.moment.caption">
          {{ resolveLocalized(item.moment.caption, locale) }}
        </figcaption>
      </figure>
    </div>
  </section>
</template>

<style scoped>
.moment-section {
  padding: clamp(5rem, 9vw, 8rem) max(1.25rem, calc((100vw - var(--content-max)) / 2));
  background: var(--color-bg);
}

.moment-heading {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  margin-bottom: 1rem;
  place-items: center;
  border: 1px solid var(--color-border);
  color: var(--color-muted);
}

.moment-heading svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.6;
}

.moment-grid {
  display: grid;
  grid-template-columns: 0.88fr 1.12fr;
  grid-template-rows: repeat(2, 19rem);
  gap: 1px;
  background: var(--color-border);
}

.moment-grid figure {
  position: relative;
  min-width: 0;
  min-height: 0;
  margin: 0;
  overflow: hidden;
  background: var(--color-surface);
}

.moment-grid figure:first-child {
  grid-row: 1 / 3;
}

.moment-grid img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 300ms var(--ease-standard);
}

.moment-grid figure:hover img {
  transform: scale(1.015);
}

.moment-grid figcaption {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 42rem) {
  .moment-section {
    padding-block: 4.5rem;
  }

  .moment-grid {
    grid-template-columns: 1fr 1fr;
    grid-template-rows: 18rem 12rem;
  }

  .moment-grid figure:first-child {
    grid-column: 1 / -1;
    grid-row: auto;
  }
}
</style>
