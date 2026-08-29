import type { AudioPlayerTrack } from '../../composables/useAudioPlayer'
import { createAudioPlayerController } from '../../composables/useAudioPlayer'
import AudioDock from '../../components/player/AudioDock.vue'
import { mount } from '@vue/test-utils'

const track: AudioPlayerTrack = {
  id: 'track-a',
  title: '曲目 A',
  previewSrc: '/media/a.wav',
  coverSrc: '/media/a.webp',
  platformLinks: [{
    provider: 'qq-music',
    url: 'https://y.qq.com/a',
  }],
}

describe('AudioDock', () => {
  it('没有当前曲目时不占据页面空间', () => {
    const player = createAudioPlayerController()
    const wrapper = mount(AudioDock, {
      props: { player, locale: 'zh-CN' },
    })

    expect(wrapper.find('[data-testid="audio-dock"]').exists()).toBe(false)
  })

  it('显示当前曲目、进度和稳定图标控制', async () => {
    const player = createAudioPlayerController()
    player.current.value = track
    player.status.value = 'paused'
    player.duration.value = 3
    player.currentTime.value = 1
    const toggle = vi.spyOn(player, 'toggle').mockResolvedValue()
    const wrapper = mount(AudioDock, {
      props: { player, locale: 'zh-CN' },
    })

    expect(wrapper.get('[data-testid="audio-dock"]').text()).toContain(track.title)
    expect(wrapper.get('input[type="range"]').attributes('max')).toBe('3')
    expect(wrapper.get('[aria-label="继续试听"]').attributes('type')).toBe('button')
    expect(wrapper.get('[aria-label="关闭播放器"]').attributes('type')).toBe('button')

    await wrapper.get('[aria-label="继续试听"]').trigger('click')
    expect(toggle).toHaveBeenCalledWith(track)
  })

  it('媒体错误时保留平台入口', () => {
    const player = createAudioPlayerController()
    player.current.value = track
    player.status.value = 'error'
    player.error.value = 'media-error'
    const wrapper = mount(AudioDock, {
      props: { player, locale: 'zh-CN' },
    })

    expect(wrapper.get('[data-testid="audio-error"]').attributes('role')).toBe('status')
    expect(wrapper.find('[data-testid="platform-links"]').exists()).toBe(true)
  })
})
