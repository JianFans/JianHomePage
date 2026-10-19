<script setup lang="ts">
import { ArrowUp, CalendarDays, Languages, Music2, Video } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
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
const showTop = ref(false)
const identity = ref<HTMLAnchorElement | null>(null)

/** 阈值状态不参与布局，监听仅在跨越阈值时触发界面更新。 */
function updateScroll() { showTop.value = window.scrollY > 480 }
/** 遵循减少动态效果偏好；把焦点交还始终可见的顶部入口。 */
function backToTop() {
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  window.scrollTo({ top: 0, behavior: reduced ? 'instant' : 'smooth' })
  identity.value?.focus({ preventScroll: true })
}
onMounted(() => { updateScroll(); window.addEventListener('scroll', updateScroll, { passive: true }) })
onBeforeUnmount(() => window.removeEventListener('scroll', updateScroll))

const labels = computed(() => props.locale === 'en'
  ? {
      home: 'Home',
      music: 'Music',
      video: 'Video',
      events: 'Live',
      language: '切换为中文',
      navigation: 'Primary navigation',
      top: 'Back to top',
    }
  : {
      home: '首页',
      music: '音乐',
      video: '影像',
      events: '现场',
      language: 'Switch to English',
      navigation: '主导航',
      top: '返回顶部',
    })
</script>

<template>
  <header class="site-header">
    <a
      ref="identity"
      class="site-identity"
      href="#top"
      :title="labels.home"
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
      <span class="site-header__top-slot">
        <Transition name="top-control">
          <IconButton
            v-if="showTop"
            data-testid="back-to-top"
            :label="labels.top"
            tooltip-align="end"
            @click="backToTop"
          ><ArrowUp aria-hidden="true" /></IconButton>
        </Transition>
      </span>
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
  min-height: 2.75rem;
  align-items: baseline;
  justify-content: center;
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

.site-header__top-slot { display: block; width: 2.75rem; height: 2.75rem; flex: 0 0 2.75rem; }
.top-control-enter-active, .top-control-leave-active { transition: opacity 160ms ease, transform 160ms ease; }
.top-control-enter-from, .top-control-leave-to { opacity: 0; transform: translateY(.25rem); }
@media (prefers-reduced-motion: reduce) { .top-control-enter-active, .top-control-leave-active { transition: none; } }

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
