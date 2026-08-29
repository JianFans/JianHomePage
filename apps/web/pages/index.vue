<script setup lang="ts">
import type { HeroSection } from '@yujian/schema'
import { computed } from 'vue'
import HeroShowcase from '../components/home/HeroShowcase.vue'
import LocaleNotice from '../components/site/LocaleNotice.vue'
import SiteHeader from '../components/site/SiteHeader.vue'
import { useContentSnapshot } from '../composables/useContentSnapshot'
import { useLocale } from '../composables/useLocale'
import { resolveLocalized } from '../utils/localized'

const snapshot = useContentSnapshot()
const { locale, selectLocale } = useLocale()
const heroSection = snapshot.homepage.sections.find(
  (section): section is HeroSection => section.type === 'hero',
)
const heroSlides = computed(() => {
  const ids = new Set(heroSection?.itemIds ?? [])
  return snapshot.heroSlides.filter(slide => ids.has(slide.id))
})
const brand = computed(() => resolveLocalized(snapshot.site.brand, locale.value))
const artistName = computed(() => resolveLocalized(snapshot.site.artistName, locale.value))

function toggleLocale() {
  selectLocale(locale.value === 'zh-CN' ? 'en' : 'zh-CN')
}
</script>

<template>
  <SiteHeader
    :brand="brand"
    :artist-name="artistName"
    :locale="locale"
    @toggle-locale="toggleLocale"
  />
  <HeroShowcase
    :slides="heroSlides"
    :assets="snapshot.assets"
    :locale="locale"
    :brand="brand"
    :artist-name="artistName"
  />
  <div
    class="next-section-edge"
    aria-hidden="true"
  >
    <span />
    <span />
    <span />
  </div>
  <LocaleNotice />
</template>

<style scoped>
.next-section-edge {
  display: grid;
  min-height: 5rem;
  grid-template-columns: 2fr 1fr 1fr;
  gap: 1px;
  padding: 1px;
  background: var(--color-border);
}

.next-section-edge span {
  background: var(--color-bg);
}
</style>
