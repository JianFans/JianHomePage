<script setup lang="ts">
import type { Asset, HeroSlide } from '@yujian/schema'
import {
  ArrowUpRight,
  ChevronLeft,
  ChevronRight,
  Pause,
  Play,
  Volume2,
  VolumeX,
} from '@lucide/vue'
import type { CSSProperties } from 'vue'
import { computed, onBeforeUnmount, ref } from 'vue'
import type { SupportedLocale } from '../../utils/localized'
import { resolveLocalized } from '../../utils/localized'
import FallbackImage from '../ui/FallbackImage.vue'
import IconButton from '../ui/IconButton.vue'

const props = defineProps<{
  slides: HeroSlide[]
  assets: Asset[]
  locale: SupportedLocale
  brand: string
  artistName: string
}>()

const activeIndex = ref(0)
const video = ref<HTMLVideoElement | null>(null)
const muted = ref(true)
const paused = ref(false)
const videoFailed = ref(false)
const mediaQuery = import.meta.client
  ? window.matchMedia('(prefers-reduced-motion: reduce)')
  : null
const reducedMotion = ref(mediaQuery?.matches ?? false)

const assetsById = computed(() => new Map(props.assets.map(asset => [asset.id, asset])))
const activeSlide = computed(() => props.slides[activeIndex.value])
const activeAsset = computed(() => {
  const slide = activeSlide.value
  return slide ? assetsById.value.get(slide.assetId) : undefined
})
const mobileAsset = computed(() => {
  const id = activeSlide.value?.mobileAssetId
  const asset = id ? assetsById.value.get(id) : undefined
  return asset && (asset.kind === 'image' || asset.kind === 'gif') ? asset : undefined
})
const posterAsset = computed(() => {
  const slide = activeSlide.value
  const asset = activeAsset.value
  const posterId = slide?.posterAssetId || asset?.posterAssetId
  return posterId ? assetsById.value.get(posterId) : undefined
})
const imageAsset = computed(() => {
  if (activeSlide.value?.mediaKind === 'video') {
    return reducedMotion.value || videoFailed.value ? posterAsset.value : undefined
  }
  return activeAsset.value
})
const videoEnabled = computed(() => (
  activeSlide.value?.mediaKind === 'video'
  && activeAsset.value?.kind === 'video'
  && !reducedMotion.value
  && !videoFailed.value
))
const targetHref = computed(() => {
  const target = activeSlide.value?.target
  if (!target) {
    return null
  }
  return target.kind === 'external' ? target.link.url : `#${target.contentId}`
})
const labels = computed(() => props.locale === 'en'
  ? {
      previous: 'Previous slide',
      next: 'Next slide',
      open: 'Open featured content',
      pause: 'Pause video',
      play: 'Play video',
      mute: 'Mute',
      unmute: 'Turn on sound',
      slide: 'Go to slide',
    }
  : {
      previous: '上一项',
      next: '下一项',
      open: '打开宣传内容',
      pause: '暂停视频',
      play: '播放视频',
      mute: '静音',
      unmute: '开启声音',
      slide: '前往轮播项',
    })
const focalStyle = computed<CSSProperties>(() => ({
  '--focal-x': `${activeSlide.value?.focalPoint.x ?? 50}%`,
  '--focal-y': `${activeSlide.value?.focalPoint.y ?? 50}%`,
} as CSSProperties))

function updateReducedMotion(event: MediaQueryListEvent) {
  reducedMotion.value = event.matches
}

mediaQuery?.addEventListener('change', updateReducedMotion)
onBeforeUnmount(() => mediaQuery?.removeEventListener('change', updateReducedMotion))

function goTo(index: number) {
  activeIndex.value = index
  muted.value = true
  paused.value = false
  videoFailed.value = false
}

function previous() {
  goTo((activeIndex.value - 1 + props.slides.length) % props.slides.length)
}

function next() {
  goTo((activeIndex.value + 1) % props.slides.length)
}

async function togglePlayback() {
  if (!video.value) {
    return
  }
  if (video.value.paused) {
    await video.value.play()
  } else {
    video.value.pause()
  }
  paused.value = video.value.paused
}
</script>

<template>
  <section
    id="top"
    class="hero-showcase"
    aria-roledescription="carousel"
    :aria-label="brand"
  >
    <span
      v-for="slide in slides"
      :id="slide.id"
      :key="`anchor-${slide.id}`"
      class="content-anchor"
      data-content-anchor
      aria-hidden="true"
    />
    <div
      v-if="activeSlide"
      class="hero-media"
      data-testid="hero-media"
      :style="focalStyle"
    >
      <picture v-if="imageAsset">
        <source
          v-if="mobileAsset"
          media="(max-width: 42rem)"
          :srcset="mobileAsset.src"
        >
        <FallbackImage
          :src="imageAsset.src"
          :alt="resolveLocalized(imageAsset.alt, locale)"
          width="1920"
          height="1200"
          fetchpriority="high"
        />
      </picture>
      <video
        v-else-if="videoEnabled && activeAsset"
        ref="video"
        :src="activeAsset.src"
        :poster="posterAsset?.src"
        :autoplay="activeSlide.autoplay"
        :muted="muted"
        playsinline
        loop
        preload="metadata"
        @play="paused = false"
        @pause="paused = true"
        @error="videoFailed = true"
      />
    </div>

    <div class="hero-scrim" />

    <div class="hero-copy">
      <h1>{{ brand }}</h1>
      <p class="hero-artist">
        {{ artistName }}
      </p>
      <p
        v-if="activeSlide"
        class="hero-headline"
      >
        {{ resolveLocalized(activeSlide.headline, locale) }}
      </p>
    </div>

    <div class="hero-tools">
      <IconButton
        v-if="slides.length > 1"
        :label="labels.previous"
        @click="previous"
      >
        <ChevronLeft aria-hidden="true" />
      </IconButton>
      <IconButton
        v-if="slides.length > 1"
        data-testid="hero-next"
        :label="labels.next"
        @click="next"
      >
        <ChevronRight aria-hidden="true" />
      </IconButton>
      <IconButton
        v-if="videoEnabled"
        :label="paused ? labels.play : labels.pause"
        :pressed="!paused"
        @click="togglePlayback"
      >
        <Play
          v-if="paused"
          aria-hidden="true"
        />
        <Pause
          v-else
          aria-hidden="true"
        />
      </IconButton>
      <IconButton
        v-if="videoEnabled"
        :label="muted ? labels.unmute : labels.mute"
        :pressed="!muted"
        @click="muted = !muted"
      >
        <VolumeX
          v-if="muted"
          aria-hidden="true"
        />
        <Volume2
          v-else
          aria-hidden="true"
        />
      </IconButton>
      <a
        v-if="targetHref"
        class="hero-link"
        :href="targetHref"
        :target="activeSlide?.target?.kind === 'external' ? '_blank' : undefined"
        :rel="activeSlide?.target?.kind === 'external' ? 'noopener noreferrer' : undefined"
        :aria-label="labels.open"
        :title="labels.open"
      >
        <ArrowUpRight aria-hidden="true" />
      </a>
    </div>

    <div
      v-if="slides.length > 1"
      class="hero-progress"
    >
      <button
        v-for="(slide, index) in slides"
        :key="slide.id"
        type="button"
        data-hero-progress
        :aria-label="`${labels.slide} ${index + 1}`"
        :aria-current="index === activeIndex"
        @click="goTo(index)"
      />
    </div>
  </section>
</template>

<style scoped>
.hero-showcase {
  position: relative;
  min-height: calc(100svh - 3.5rem);
  max-height: 58rem;
  overflow: hidden;
  isolation: isolate;
  background: var(--color-surface);
}

.content-anchor {
  position: absolute;
  top: 0;
  left: 0;
  width: 1px;
  height: 1px;
  pointer-events: none;
  scroll-margin-top: 3.5rem;
}

.hero-media,
.hero-media picture,
.hero-media img,
.hero-media video,
.hero-scrim {
  position: absolute;
  inset: 0;
  width: 100%;
  height: 100%;
}

.hero-media picture {
  display: block;
}

.hero-media img,
.hero-media video {
  display: block;
  object-fit: cover;
  object-position: var(--focal-x) var(--focal-y);
}

.hero-scrim {
  z-index: 1;
  background: rgba(6, 8, 9, 0.2);
}

.hero-scrim::before {
  position: absolute;
  inset: 0 auto 0 0;
  width: 58%;
  background: rgba(6, 8, 9, 0.54);
  content: "";
}

.hero-copy {
  position: absolute;
  z-index: 2;
  bottom: clamp(7.5rem, 17vh, 10.5rem);
  left: max(1.25rem, calc((100vw - var(--content-max)) / 2));
  width: min(32rem, calc(100% - 2.5rem));
}

.hero-copy h1,
.hero-copy p {
  margin: 0;
  letter-spacing: 0;
}

.hero-copy h1 {
  font-size: clamp(2.75rem, 5.5rem, 5.5rem);
  font-weight: 520;
  line-height: 0.96;
}

.hero-artist {
  margin-top: 1rem !important;
  color: var(--color-text);
  font-size: 1rem;
  font-weight: 520;
}

.hero-headline {
  margin-top: 0.4rem !important;
  color: var(--color-muted);
  font-size: 0.875rem;
}

.hero-tools {
  position: absolute;
  z-index: 3;
  right: max(1.25rem, calc((100vw - var(--content-max)) / 2));
  bottom: 2.5rem;
  display: flex;
  gap: 0.5rem;
}

.hero-link {
  display: grid;
  width: 2.75rem;
  height: 2.75rem;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--color-border) 72%, transparent);
  border-radius: var(--radius-tool);
  background: color-mix(in srgb, var(--color-bg) 72%, transparent);
}

.hero-link:hover {
  border-color: var(--color-accent);
  background: var(--color-surface-raised);
}

.hero-link svg {
  width: 1.125rem;
  height: 1.125rem;
  stroke-width: 1.75;
}

.hero-progress {
  position: absolute;
  z-index: 3;
  bottom: 1.35rem;
  left: max(1.25rem, calc((100vw - var(--content-max)) / 2));
  display: flex;
  gap: 0.15rem;
}

.hero-progress button {
  position: relative;
  width: 2.75rem;
  height: 2.75rem;
  padding: 0;
  border: 0;
  border-radius: 0;
  background: transparent;
  cursor: pointer;
}

.hero-progress button::before {
  position: absolute;
  top: 50%;
  left: 50%;
  width: 2rem;
  height: 0.25rem;
  transform: translate(-50%, -50%);
  background: var(--color-muted);
  content: "";
  opacity: 0.4;
}

.hero-progress button[aria-current="true"]::before {
  background: var(--color-text);
  opacity: 1;
}

@media (max-width: 42rem) {
  .hero-showcase {
    min-height: calc(100svh - 3rem);
    max-height: none;
  }

  .hero-scrim {
    background: rgba(6, 8, 9, 0.18);
  }

  .hero-scrim::before {
    inset: 52% 0 0;
    width: 100%;
    background: rgba(6, 8, 9, 0.68);
  }

  .hero-copy {
    bottom: 8.5rem;
  }

  .hero-copy h1 {
    font-size: 3.25rem;
  }

  .hero-tools {
    right: 1.25rem;
    bottom: 1.35rem;
  }

  .hero-progress {
    left: 1.25rem;
    bottom: 0.75rem;
  }
}
</style>
