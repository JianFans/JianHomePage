import { createAudioPlayerController } from '../composables/useAudioPlayer'

export default defineNuxtPlugin(() => ({
  provide: {
    audioPlayer: createAudioPlayerController(),
  },
}))
