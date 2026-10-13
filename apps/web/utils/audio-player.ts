import type { LocalizedText, PlatformLink } from '@yujian/schema'
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

/** Each controller owns its state so isolated previews cannot change public playback. */
export function createAudioPlayerState() {
  const current = ref<AudioPlayerTrack | null>(null)
  const status = ref<AudioPlayerStatus>('idle')
  const error = ref<string | null>(null)

  /** A newly accepted track clears errors from the previous selection. */
  function play(track: AudioPlayerTrack) {
    current.value = track
    status.value = 'playing'
    error.value = null
  }

  /** Retain the current track so a failed preview still exposes its platform links. */
  function fail(reason: string) {
    status.value = 'error'
    error.value = reason
  }

  /** Return the visible state to idle without retaining track or error references. */
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

/** Lazily owns one media instance and ignores events from superseded tracks or requests. */
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

  /** Scope every listener to its media generation so old events cannot replace current state. */
  function bindMedia(activeMedia: AudioMedia, generation: number): MediaListeners {
    /** Identity and generation both matter when a detached media dispatches a queued event. */
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

  /** Invalidate listeners before pausing and clearing the source, which may dispatch events. */
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

  /** Delay browser media creation until playback, keeping construction safe during SSR. */
  function getMedia() {
    if (!media) {
      media = createAudio()
      media.preload = 'metadata'
      const generation = ++mediaGeneration
      mediaListeners = bindMedia(media, generation)
    }
    return media
  }

  /** Only the latest request may finish loading; switching tracks releases the previous media. */
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

  /** Invalidate pending play promises before pausing so they cannot restore playing state. */
  function pause() {
    playbackRequest++
    media?.pause()
    if (state.current.value && state.status.value !== 'error') {
      state.status.value = 'paused'
    }
  }

  /** Resume the retained media while rejecting state changes from superseded requests. */
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

  /** Preserve first-occurrence queue order and toggle the current track without recreating media. */
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

  /** Queue boundaries are terminal; navigation never wraps to an unexpected preview. */
  async function previous() {
    if (!canPrevious.value) {
      return
    }
    await play(queue.value[currentQueueIndex.value - 1]!)
  }

  /** Advance only when another queued track exists, retaining ordinary play race protection. */
  async function next() {
    if (!canNext.value) {
      return
    }
    await play(queue.value[currentQueueIndex.value + 1]!)
  }

  /** Clamp seeking to known duration, allowing a nonnegative target before metadata arrives. */
  function seek(nextTime: number) {
    if (!media) {
      return
    }
    const maximum = duration.value || nextTime
    media.currentTime = Math.min(Math.max(nextTime, 0), maximum)
    currentTime.value = media.currentTime
  }

  /** Release media and queue ownership and invalidate every outstanding playback request. */
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
