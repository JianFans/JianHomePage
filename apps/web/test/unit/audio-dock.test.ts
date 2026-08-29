import type { AudioPlayerTrack } from '../../composables/useAudioPlayer'
import { mount } from '@vue/test-utils'
import AudioDock from '../../components/player/AudioDock.vue'
import { createAudioPlayerController } from '../../composables/useAudioPlayer'

const tracks: AudioPlayerTrack[] = [
  {
    id: 'track-a',
    title: {
      'zh-CN': '曲目 A',
      en: 'Track A',
    },
    previewSrc: '/media/a.wav',
    platformLinks: [],
  },
  {
    id: 'track-b',
    title: {
      'zh-CN': '曲目 B',
      en: 'Track B',
    },
    previewSrc: '/media/b.wav',
    platformLinks: [],
  },
]

function createFakeAudio() {
  return {
    src: '',
    preload: '',
    currentTime: 0,
    duration: 3,
    paused: true,
    play: vi.fn(async () => {}),
    pause: vi.fn(),
    load: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }
}

describe('AudioDock', () => {
  it('显示可访问的队列导航并反映边界状态', async () => {
    const player = createAudioPlayerController({ createAudio: createFakeAudio })
    await player.toggle(tracks[0]!, tracks)
    const wrapper = mount(AudioDock, {
      props: { player, locale: 'zh-CN' },
    })

    expect(wrapper.get('[aria-label="上一首"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[aria-label="下一首"]').attributes('disabled')).toBeUndefined()

    await wrapper.get('[aria-label="下一首"]').trigger('click')
    expect(player.current.value?.id).toBe('track-b')
  })

  it('切换语言后同步更新当前曲目标题', async () => {
    const player = createAudioPlayerController({ createAudio: createFakeAudio })
    await player.toggle(tracks[0]!, tracks)
    const wrapper = mount(AudioDock, {
      props: { player, locale: 'zh-CN' },
    })

    expect(wrapper.get('[data-testid="audio-dock"]').attributes('aria-label')).toBe('曲目 A')
    expect(wrapper.get('.audio-dock__track strong').text()).toBe('曲目 A')

    await wrapper.setProps({ locale: 'en' })

    expect(wrapper.get('[data-testid="audio-dock"]').attributes('aria-label')).toBe('Track A')
    expect(wrapper.get('.audio-dock__track strong').text()).toBe('Track A')
  })
})
