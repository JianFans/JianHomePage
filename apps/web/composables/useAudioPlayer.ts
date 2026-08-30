import type { LocalizedText, PlatformLink } from '@yujian/schema'
import { useNuxtApp } from '#app'
import { computed, ref } from 'vue'

export type AudioPlayerStatus = 'idle' | 'loading' | 'playing' | 'paused' | 'error'

export interface AudioPlayerTrack {
  id: string
  title: LocalizedText
  previewSrc: string
  coverSrc?: string
  platformLinks: PlatformLink[]
}

interface AudioMedia {
  src: string
  preload: string
  currentTime: number
  duration: number
  paused: boolean
  play: () => Promise<void>
  pause: () => void
  load: () => void
  addEventListener: (name: string, listener: () => void) => void
  removeEventListener: (name: string, listener: () => void) => void
}

interface AudioPlayerControllerOptions {
  createAudio?: () => AudioMedia
}

export function createAudioPlayerState() {
  const current = ref<AudioPlayerTrack | null>(null)
  const status = ref<AudioPlayerStatus>('idle')
  const error = ref<string | null>(null)

  function play(track: AudioPlayerTrack) {
    current.value = track
    status.value = 'playing'
    error.value = null
  }

  function fail(reason: string) {
    status.value = 'error'
    error.value = reason
  }

  function close() {
    current.value = null
    status.value = 'idle'
    error.value = null
  }

  return {
    current,
    status,
    error,
    play,
    fail,
    close,
  }
}

export function createAudioPlayerController({
  createAudio = () => new Audio(),
}: AudioPlayerControllerOptions = {}) {
  const state = createAudioPlayerState()
  const currentTime = ref(0)
  const duration = ref(0)
  const queue = ref<AudioPlayerTrack[]>([])
  const currentQueueIndex = computed(() => queue.value.findIndex(
    track => track.id === state.current.value?.id,
  ))
  const canPrevious = computed(() => currentQueueIndex.value > 0)
  const canNext = computed(() => (
    currentQueueIndex.value >= 0
    && currentQueueIndex.value < queue.value.length - 1
  ))
  let media: AudioMedia | null = null
  let mediaGeneration = 0
  let playbackRequest = 0

  type MediaListeners = Record<'error' | 'timeupdate' | 'loadedmetadata' | 'ended', () => void>
  let mediaListeners: MediaListeners | null = null

  function bindMedia(activeMedia: AudioMedia, generation: number): MediaListeners {
    const isActive = () => media === activeMedia && mediaGeneration === generation
    const listeners: MediaListeners = {
      error: () => {
        if (isActive()) state.fail('media-error')
      },
      timeupdate: () => {
        if (isActive()) currentTime.value = activeMedia.currentTime
      },
      loadedmetadata: () => {
        if (isActive()) {
          duration.value = Number.isFinite(activeMedia.duration) ? activeMedia.duration : 0
        }
      },
      ended: () => {
        if (!isActive()) return
        activeMedia.currentTime = 0
        currentTime.value = 0
        state.status.value = 'paused'
      },
    }
    for (const [name, listener] of Object.entries(listeners)) {
      activeMedia.addEventListener(name, listener)
    }
    return listeners
  }

  function disposeMedia() {
    const activeMedia = media
    const listeners = mediaListeners
    if (!activeMedia) return

    mediaGeneration++
    media = null
    mediaListeners = null
    if (listeners) {
      for (const [name, listener] of Object.entries(listeners)) {
        activeMedia.removeEventListener(name, listener)
      }
    }
    activeMedia.pause()
    activeMedia.src = ''
    activeMedia.load()
  }

  function getMedia() {
    if (!media) {
      media = createAudio()
      media.preload = 'metadata'
      const generation = ++mediaGeneration
      mediaListeners = bindMedia(media, generation)
    }
    return media
  }

  async function play(track: AudioPlayerTrack) {
    const request = ++playbackRequest
    if (!queue.value.some(item => item.id === track.id)) {
      queue.value = [track]
    }
    if (state.current.value && state.current.value.id !== track.id) {
      disposeMedia()
      currentTime.value = 0
      duration.value = 0
    }
    const activeMedia = getMedia()

    state.current.value = track
    state.status.value = 'loading'
    state.error.value = null
    activeMedia.src = track.previewSrc

    try {
      await activeMedia.play()
      if (request !== playbackRequest) return
      state.status.value = 'playing'
    } catch {
      if (request !== playbackRequest) return
      state.fail('playback-rejected')
    }
  }

  function pause() {
    playbackRequest++
    media?.pause()
    if (state.current.value && state.status.value !== 'error') {
      state.status.value = 'paused'
    }
  }

  async function resume() {
    if (!media) {
      return
    }
    const request = ++playbackRequest
    state.status.value = 'loading'
    try {
      await media.play()
      if (request !== playbackRequest) return
      state.status.value = 'playing'
    } catch {
      if (request !== playbackRequest) return
      state.fail('playback-rejected')
    }
  }

  async function toggle(track: AudioPlayerTrack, nextQueue?: AudioPlayerTrack[]) {
    if (nextQueue?.length) {
      queue.value = nextQueue.filter((item, index, items) => (
        items.findIndex(candidate => candidate.id === item.id) === index
      ))
    }
    if (state.current.value?.id !== track.id) {
      await play(track)
      return
    }
    if (state.status.value === 'playing' || state.status.value === 'loading') {
      pause()
      return
    }
    await resume()
  }

  async function previous() {
    if (!canPrevious.value) {
      return
    }
    await play(queue.value[currentQueueIndex.value - 1]!)
  }

  async function next() {
    if (!canNext.value) {
      return
    }
    await play(queue.value[currentQueueIndex.value + 1]!)
  }

  function seek(nextTime: number) {
    if (!media) {
      return
    }
    const maximum = duration.value || nextTime
    media.currentTime = Math.min(Math.max(nextTime, 0), maximum)
    currentTime.value = media.currentTime
  }

  function close() {
    playbackRequest++
    disposeMedia()
    state.close()
    queue.value = []
    currentTime.value = 0
    duration.value = 0
  }

  return {
    ...state,
    currentTime,
    duration,
    queue,
    canPrevious,
    canNext,
    play,
    pause,
    resume,
    toggle,
    previous,
    next,
    seek,
    close,
  }
}

export type AudioPlayerController = ReturnType<typeof createAudioPlayerController>

export function useAudioPlayer(): AudioPlayerController {
  return useNuxtApp().$audioPlayer
}
