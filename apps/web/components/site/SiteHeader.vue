<script setup lang="ts">
import { CalendarDays, Languages, Music2, Video } from '@lucide/vue'
import { computed } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import IconButton from '../ui/IconButton.vue'

const props = defineProps<{
  brand: string
  artistName: string
  locale: SupportedLocale
}>()

const emit = defineEmits<{
  toggleLocale: []
}>()

const labels = computed(() => props.locale === 'en'
  ? {
      home: 'Home',
      music: 'Music',
      video: 'Video',
      events: 'Live',
      language: '切换为中文',
      navigation: 'Primary navigation',
    }
  : {
      home: '首页',
      music: '音乐',
      video: '影像',
      events: '现场',
      language: 'Switch to English',
      navigation: '主导航',
    })
</script>

<template>
  <header class="site-header">
    <a
      class="site-identity"
      href="#top"
      :aria-label="labels.home"
    >
      <span>{{ brand }}</span>
      <small>{{ artistName }}</small>
    </a>

    <nav :aria-label="labels.navigation">
      <a
        href="#music"
        :aria-label="labels.music"
        :title="labels.music"
      >
        <Music2 aria-hidden="true" />
      </a>
      <a
        href="#video"
        :aria-label="labels.video"
        :title="labels.video"
      >
        <Video aria-hidden="true" />
      </a>
      <a
        href="#event"
        :aria-label="labels.events"
        :title="labels.events"
      >
        <CalendarDays aria-hidden="true" />
      </a>
      <IconButton
        :label="labels.language"
        tooltip-align="end"
        @click="emit('toggleLocale')"
      >
        <Languages aria-hidden="true" />
      </IconButton>
    </nav>
  </header>
</template>

<style scoped>
.site-header {
  position: fixed;
  z-index: 30;
  top: 0;
  left: 0;
  display: flex;
  width: 100%;
  height: var(--header-height);
  align-items: center;
  justify-content: space-between;
  padding: 0 max(1.25rem, calc((100vw - var(--content-max)) / 2));
  border-bottom: 1px solid color-mix(in srgb, var(--color-border) 52%, transparent);
  background: color-mix(in srgb, var(--color-bg) 82%, transparent);
  backdrop-filter: blur(14px);
}

.site-identity {
  display: flex;
  align-items: baseline;
  gap: 0.7rem;
  text-decoration: none;
}

.site-identity span {
  font-size: 1rem;
  font-weight: 650;
}

.site-identity small {
  color: var(--color-muted);
  font-size: 0.72rem;
}

nav {
  display: flex;
  align-items: center;
  gap: 0.35rem;
}

nav > a {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  border: 1px solid transparent;
  border-radius: var(--radius-tool);
  color: var(--color-muted);
}

nav > a:hover,
nav > a:focus-visible {
  border-color: var(--color-border);
  color: var(--color-text);
}

nav > a svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.7;
}

@media (max-width: 34rem) {
  .site-header {
    padding-inline: 0.8rem;
  }

  .site-identity small,
  nav > a:nth-child(3) {
    display: none;
  }
}
</style>
