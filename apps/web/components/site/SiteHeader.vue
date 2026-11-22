<script setup lang="ts">
import { ArrowUp, CalendarDays, Images, Languages, Music2, Video } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import IconButton from '../ui/IconButton.vue'

const props = withDefaults(defineProps<{
  brand: string
  artistName: string
  locale: SupportedLocale
  homeHref?: string
  musicHref?: string
  musicCurrent?: boolean
  videoHref?: string
  videoCurrent?: boolean
  eventsHref?: string
  eventsCurrent?: boolean
  momentsHref?: string
  momentsCurrent?: boolean
  showVideo?: boolean
  showEvents?: boolean
}>(), {
  homeHref: '',
  musicHref: '',
  videoHref: '',
  eventsHref: '',
  momentsHref: '/moments/',
  showVideo: true,
  showEvents: true,
  musicCurrent: false,
  videoCurrent: false,
  eventsCurrent: false,
  momentsCurrent: false,
})

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
      moments: 'Moments',
      language: '切换为中文',
      navigation: 'Primary navigation',
      top: 'Back to top',
    }
  : {
      home: '首页',
      music: '音乐',
      video: '影像',
      events: '现场',
      moments: '片段',
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
      :href="homeHref || '#top'"
      :title="labels.home"
    >
      <span>{{ brand }}</span>
      <small>{{ artistName }}</small>
    </a>

    <nav :aria-label="labels.navigation">
      <a
        :href="musicHref || '#music'"
        :aria-current="musicCurrent ? 'page' : undefined"
        :aria-label="labels.music"
        :title="labels.music"
      >
        <Music2 aria-hidden="true" />
      </a>
      <a
        v-if="showVideo"
        :href="videoHref || `${homeHref || ''}#video`"
        :aria-current="videoCurrent ? 'page' : undefined"
        :aria-label="labels.video"
        :title="labels.video"
      >
        <Video aria-hidden="true" />
      </a>
      <a
        v-if="showEvents"
        class="site-header__events"
        :class="{ 'site-header__events--current': eventsCurrent }"
        :href="eventsHref || `${homeHref || ''}#event`"
        :aria-current="eventsCurrent ? 'page' : undefined"
        :aria-label="labels.events"
        :title="labels.events"
      >
        <CalendarDays aria-hidden="true" />
      </a>
      <a
        class="site-header__moments"
        :class="{ 'site-header__moments--current': momentsCurrent }"
        :href="momentsHref"
        :aria-current="momentsCurrent ? 'page' : undefined"
        :aria-label="labels.moments"
        :title="labels.moments"
      >
        <Images aria-hidden="true" />
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
nav > a:focus-visible,
nav > a[aria-current="page"] {
  border-color: var(--color-border);
  color: var(--color-text);
}

nav > a svg {
  width: 1.1rem;
  height: 1.1rem;
  stroke-width: 1.7;
}

.site-header__top-slot { display: block; width: 2.75rem; height: 2.75rem; flex: 0 0 2.75rem; }
.top-control-enter-active, .top-control-leave-active { transition: opacity var(--motion-normal) var(--motion-ease-out), transform var(--motion-normal) var(--motion-ease-out); }
.top-control-enter-from, .top-control-leave-to { opacity: 0; transform: translateY(.25rem); }
@media (prefers-reduced-motion: reduce) { .top-control-enter-active, .top-control-leave-active { transition: none; } }

@media (max-width: 34rem) {
  .site-header {
    padding-inline: 0.8rem;
  }

  .site-identity small,
  .site-header__events:not(.site-header__events--current),
  .site-header__moments:not(.site-header__moments--current) {
    display: none;
  }
}
</style>
