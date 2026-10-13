import { useNuxtApp } from '#app'
import type { AudioPlayerController } from '../utils/audio-player'

// Star re-export keeps Nuxt's auto-import scanner pointed at the original definitions.
export * from '../utils/audio-player'

/** Public pages share the Nuxt plugin's controller throughout the application. */
export function useAudioPlayer(): AudioPlayerController {
  return useNuxtApp().$audioPlayer
}
