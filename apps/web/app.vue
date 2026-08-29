<script setup lang="ts">
import AudioDock from './components/player/AudioDock.vue'
import { useAudioPlayer } from './composables/useAudioPlayer'
import { useLocale } from './composables/useLocale'

const player = useAudioPlayer()
const { locale } = useLocale()
</script>

<template>
  <main>
    <NuxtPage />
  </main>
  <div data-testid="audio-dock-host">
    <AudioDock
      :player="player"
      :locale="locale"
    />
  </div>
</template>
