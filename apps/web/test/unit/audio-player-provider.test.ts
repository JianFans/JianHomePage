import { mountSuspended } from '@nuxt/test-utils/runtime'
import { defineComponent, h } from 'vue'
import { useAudioPlayer } from '../../composables/useAudioPlayer'

describe('全局音频播放器', () => {
  it('同一 Nuxt 应用内复用同一个控制器', async () => {
    const Probe = defineComponent({
      setup() {
        const first = useAudioPlayer()
        const second = useAudioPlayer()
        return () => h('span', {
          'data-testid': 'same-player',
          'data-same': String(first === second),
        })
      },
    })

    const wrapper = await mountSuspended(Probe)

    expect(wrapper.get('[data-testid="same-player"]').attributes('data-same')).toBe('true')
  })
})
