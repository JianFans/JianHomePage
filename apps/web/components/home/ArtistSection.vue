<script setup lang="ts">
import type { ArtistProfile, Asset } from '@yujian/schema'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  artist: ArtistProfile
  assets: Asset[]
  locale: SupportedLocale
}>()

const portrait = computed(() => props.assets.find(asset => asset.id === props.artist.portraitAssetId))
</script>

<template>
  <section
    id="artist"
    class="artist-section"
    :aria-labelledby="`${artist.id}-name`"
  >
    <img
      v-if="portrait"
      class="artist-texture"
      :src="portrait.src"
      :alt="resolveLocalized(portrait.alt, locale)"
      :width="portrait.width"
      :height="portrait.height"
      loading="lazy"
    >
    <div class="artist-scrim" />
    <div class="artist-inner">
      <h2 :id="`${artist.id}-name`">
        {{ resolveLocalized(artist.name, locale) }}
      </h2>
      <p>{{ resolveLocalized(artist.shortBio, locale) }}</p>
      <PlatformLinks
        :links="artist.platformLinks"
        :locale="locale"
        menu-align="start"
      />
    </div>
  </section>
</template>

<style scoped>
.artist-section {
  position: relative;
  min-height: 32rem;
  overflow: hidden;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface);
  isolation: isolate;
}

.artist-texture,
.artist-scrim {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.artist-texture {
  z-index: -2;
  object-fit: cover;
  object-position: 64% 50%;
}

.artist-scrim {
  z-index: -1;
  background: rgba(6, 8, 9, 0.64);
}

.artist-scrim::before {
  position: absolute;
  inset: 0 42% 0 0;
  background: rgba(6, 8, 9, 0.42);
  content: "";
}

.artist-inner {
  display: grid;
  min-height: 32rem;
  max-width: var(--content-max);
  align-content: center;
  gap: 1rem;
  padding: 5rem 1.25rem;
  margin-inline: auto;
}

.artist-inner h2,
.artist-inner p {
  width: min(32rem, 100%);
  margin: 0;
  letter-spacing: 0;
}

.artist-inner h2 {
  font-size: 2.4rem;
  font-weight: 540;
}

.artist-inner p {
  color: var(--color-muted);
  font-size: 0.88rem;
  line-height: 1.75;
}

.artist-inner :deep(.platform-links) {
  margin-top: 0.75rem;
}

@media (max-width: 42rem) {
  .artist-section,
  .artist-inner {
    min-height: 27rem;
  }

  .artist-texture {
    object-position: 52% 50%;
  }

  .artist-scrim {
    background: rgba(6, 8, 9, 0.76);
  }

  .artist-scrim::before {
    display: none;
  }

  .artist-inner h2 {
    font-size: 2rem;
  }
}
</style>
