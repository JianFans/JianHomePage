<script setup lang="ts">
import { AlertCircle, LoaderCircle, Pause, Play, X } from '@lucide/vue'
import { computed } from 'vue'
import type { AudioPlayerController } from '../../composables/useAudioPlayer'
import type { SupportedLocale } from '../../utils/localized'
import IconButton from '../ui/IconButton.vue'
import PlatformLinks from '../ui/PlatformLinks.vue'

const props = defineProps<{
  player: AudioPlayerController
  locale: SupportedLocale
}>()

const current = computed(() => props.player.current.value)
const status = computed(() => props.player.status.value)
const currentTime = computed(() => props.player.currentTime.value)
const duration = computed(() => props.player.duration.value)
const labels = computed(() => props.locale === 'en'
  ? {
      pause: 'Pause preview',
      resume: 'Resume preview',
      close: 'Close player',
      progress: 'Preview progress',
      error: 'Preview unavailable. Open on a music platform.',
    }
  : {
      pause: '暂停试听',
      resume: '继续试听',
      close: '关闭播放器',
      progress: '试听进度',
      error: '试听暂不可用，请前往音乐平台。',
    })

function updateProgress(event: Event) {
  props.player.seek(Number((event.target as HTMLInputElement).value))
}
</script>

<template>
  <Transition name="audio-dock">
    <aside
      v-if="current"
      class="audio-dock"
      data-testid="audio-dock"
      :aria-label="current.title"
    >
      <img
        v-if="current.coverSrc"
        class="audio-dock__cover"
        :src="current.coverSrc"
        alt=""
        width="56"
        height="56"
      >

      <div class="audio-dock__track">
        <strong>{{ current.title }}</strong>
        <input
          type="range"
          min="0"
          :max="duration || 0"
          step="0.05"
          :value="currentTime"
          :aria-label="labels.progress"
          @input="updateProgress"
        >
      </div>

      <div class="audio-dock__actions">
        <span
          v-if="status === 'error'"
          class="audio-dock__error"
          data-testid="audio-error"
          role="status"
          :aria-label="labels.error"
          :title="labels.error"
        >
          <AlertCircle aria-hidden="true" />
        </span>

        <PlatformLinks
          v-if="status === 'error'"
          :links="current.platformLinks"
          :locale="locale"
        />

        <IconButton
          v-if="status !== 'error'"
          :label="status === 'playing' ? labels.pause : labels.resume"
          :pressed="status === 'playing'"
          @click="player.toggle(current)"
        >
          <LoaderCircle
            v-if="status === 'loading'"
            class="audio-dock__loading"
            aria-hidden="true"
          />
          <Pause
            v-else-if="status === 'playing'"
            aria-hidden="true"
          />
          <Play
            v-else
            aria-hidden="true"
          />
        </IconButton>

        <IconButton
          :label="labels.close"
          tooltip-align="end"
          @click="player.close"
        >
          <X aria-hidden="true" />
        </IconButton>
      </div>
    </aside>
  </Transition>
</template>

<style scoped>
.audio-dock {
  position: fixed;
  z-index: 45;
  right: max(1rem, calc((100vw - var(--content-max)) / 2));
  bottom: 1rem;
  left: max(1rem, calc((100vw - var(--content-max)) / 2));
  display: grid;
  min-height: 4.5rem;
  grid-template-columns: 3.5rem minmax(8rem, 1fr) auto;
  align-items: center;
  gap: 0.8rem;
  padding: 0.45rem;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-tool);
  background: color-mix(in srgb, var(--color-surface-raised) 94%, transparent);
  box-shadow: 0 1.25rem 3rem rgba(0, 0, 0, 0.42);
  backdrop-filter: blur(18px);
}

.audio-dock__cover {
  display: block;
  width: 3.5rem;
  height: 3.5rem;
  object-fit: cover;
}

.audio-dock__track {
  display: grid;
  min-width: 0;
  gap: 0.4rem;
}

.audio-dock__track strong {
  overflow: hidden;
  font-size: 0.82rem;
  font-weight: 560;
  letter-spacing: 0;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.audio-dock__track input {
  width: 100%;
  height: 1rem;
  margin: 0;
  accent-color: var(--color-warm);
  cursor: pointer;
}

.audio-dock__actions {
  display: flex;
  align-items: center;
  gap: 0.4rem;
}

.audio-dock__error {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  color: var(--color-warm);
}

.audio-dock__error svg {
  width: 1.1rem;
  height: 1.1rem;
}

.audio-dock__loading {
  animation: audio-loading 900ms linear infinite;
}

.audio-dock-enter-active,
.audio-dock-leave-active {
  transition: opacity 160ms var(--ease-standard), transform 160ms var(--ease-standard);
}

.audio-dock-enter-from,
.audio-dock-leave-to {
  transform: translateY(0.75rem);
  opacity: 0;
}

@keyframes audio-loading {
  to {
    transform: rotate(360deg);
  }
}

@media (max-width: 42rem) {
  .audio-dock {
    right: 0.6rem;
    bottom: 0.6rem;
    left: 0.6rem;
    min-height: 4rem;
    grid-template-columns: 3rem minmax(0, 1fr) auto;
    gap: 0.55rem;
  }

  .audio-dock__cover {
    width: 3rem;
    height: 3rem;
  }

  .audio-dock__actions {
    gap: 0.25rem;
  }

  .audio-dock__actions :deep(.platform-links) {
    display: none;
  }
}
</style>
