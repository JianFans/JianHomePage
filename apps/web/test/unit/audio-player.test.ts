import type { AudioPlayerTrack } from '../../composables/useAudioPlayer'
import {
  createAudioPlayerController,
  createAudioPlayerState,
} from '../../composables/useAudioPlayer'

const trackA: AudioPlayerTrack = {
  id: 'track-a',
  title: {
    'zh-CN': '曲目 A',
    en: 'Track A',
  },
  previewSrc: '/media/a.wav',
  platformLinks: [{
    provider: 'qq-music',
    url: 'https://y.qq.com/a',
  }],
}

const trackB: AudioPlayerTrack = {
  id: 'track-b',
  title: {
    'zh-CN': '曲目 B',
    en: 'Track B',
  },
  previewSrc: '/media/b.wav',
  platformLinks: [{
    provider: 'netease-music',
    url: 'https://music.163.com/b',
  }],
}

class FakeAudio {
  src = ''
  preload = ''
  currentTime = 0
  duration = 3
  paused = true
  pauseCalls = 0
  loadCalls = 0
  private listeners = new Map<string, Set<() => void>>()

  async play() {
    this.paused = false
    this.emit('play')
  }

  pause() {
    this.paused = true
    this.pauseCalls += 1
    this.emit('pause')
  }

  load() {
    this.loadCalls += 1
  }

  addEventListener(name: string, listener: () => void) {
    const listeners = this.listeners.get(name) ?? new Set()
    listeners.add(listener)
    this.listeners.set(name, listeners)
  }

  removeEventListener(name: string, listener: () => void) {
    this.listeners.get(name)?.delete(listener)
  }

  emit(name: string) {
    this.listeners.get(name)?.forEach(listener => listener())
  }
}

describe('音频播放器状态', () => {
  it('同一时间只保留一个当前曲目', () => {
    const player = createAudioPlayerState()

    player.play(trackA)
    player.play(trackB)

    expect(player.current.value?.id).toBe(trackB.id)
    expect(player.status.value).toBe('playing')
  })

  it('失败后保留平台链接并清除播放状态', () => {
    const player = createAudioPlayerState()

    player.play(trackA)
    player.fail('media-error')

    expect(player.status.value).toBe('error')
    expect(player.current.value?.platformLinks).toEqual(trackA.platformLinks)
    expect(player.error.value).toBe('media-error')
  })

  it('关闭后释放当前曲目并回到空闲状态', () => {
    const player = createAudioPlayerState()

    player.play(trackA)
    player.close()

    expect(player.current.value).toBeNull()
    expect(player.status.value).toBe('idle')
  })
})

describe('音频播放器控制器', () => {
  it('首次播放前不创建媒体实例', async () => {
    const media = new FakeAudio()
    const createAudio = vi.fn(() => media)
    const player = createAudioPlayerController({ createAudio })

    expect(createAudio).not.toHaveBeenCalled()

    await player.play(trackA)

    expect(createAudio).toHaveBeenCalledTimes(1)
    expect(media.src).toContain(trackA.previewSrc)
  })

  it('切换曲目时暂停旧媒体并复用单个实例', async () => {
    const media = new FakeAudio()
    const createAudio = vi.fn(() => media)
    const player = createAudioPlayerController({ createAudio })

    await player.play(trackA)
    await player.play(trackB)

    expect(media.pauseCalls).toBe(1)
    expect(createAudio).toHaveBeenCalledTimes(1)
    expect(player.current.value?.id).toBe(trackB.id)
  })

  it('关闭时暂停并释放媒体资源', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.play(trackA)
    player.close()

    expect(media.pauseCalls).toBe(1)
    expect(media.src).toBe('')
    expect(media.loadCalls).toBe(1)
    expect(player.current.value).toBeNull()
  })

  it('原生媒体错误会保留当前曲目的平台入口', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.play(trackA)
    media.emit('error')

    expect(player.status.value).toBe('error')
    expect(player.current.value?.platformLinks).toEqual(trackA.platformLinks)
  })

  it('重复操作当前曲目会在播放和暂停间切换', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.toggle(trackA)
    await player.toggle(trackA)

    expect(media.pauseCalls).toBe(1)
    expect(player.status.value).toBe('paused')

    await player.toggle(trackA)
    expect(player.status.value).toBe('playing')
  })

  it('同步媒体时间并允许跳转试听进度', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.play(trackA)
    media.currentTime = 1
    media.duration = 3
    media.emit('timeupdate')
    media.emit('loadedmetadata')

    expect(player.currentTime.value).toBe(1)
    expect(player.duration.value).toBe(3)

    player.seek(2)
    expect(media.currentTime).toBe(2)
  })

  it('关闭后清除试听进度', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.play(trackA)
    media.currentTime = 2
    media.duration = 3
    media.emit('timeupdate')
    media.emit('loadedmetadata')
    player.close()

    expect(player.currentTime.value).toBe(0)
    expect(player.duration.value).toBe(0)
  })

  it('试听结束后回到可重新播放状态', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.play(trackA)
    media.currentTime = 3
    media.emit('ended')

    expect(player.status.value).toBe('paused')
    expect(player.currentTime.value).toBe(0)
    expect(media.currentTime).toBe(0)
  })

  it('在试听队列中提供上一首和下一首导航', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.toggle(trackA, [trackA, trackB])

    expect(player.canPrevious.value).toBe(false)
    expect(player.canNext.value).toBe(true)

    await player.next()
    expect(player.current.value?.id).toBe(trackB.id)
    expect(player.canPrevious.value).toBe(true)
    expect(player.canNext.value).toBe(false)

    await player.previous()
    expect(player.current.value?.id).toBe(trackA.id)
  })

  it('队列边界不会循环或替换当前曲目', async () => {
    const media = new FakeAudio()
    const player = createAudioPlayerController({ createAudio: () => media })

    await player.toggle(trackA, [trackA, trackB])
    await player.previous()
    expect(player.current.value?.id).toBe(trackA.id)

    await player.next()
    await player.next()
    expect(player.current.value?.id).toBe(trackB.id)
  })
})
