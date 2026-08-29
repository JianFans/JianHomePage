import { mountSuspended } from '@nuxt/test-utils/runtime'
import App from '../../app.vue'

describe('应用壳', () => {
  it('提供主内容区域和全局播放器挂载点', async () => {
    const wrapper = await mountSuspended(App)

    expect(wrapper.find('main').exists()).toBe(true)
    expect(wrapper.find('[data-testid="audio-dock-host"]').exists()).toBe(true)
  })
})
