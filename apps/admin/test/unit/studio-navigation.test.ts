import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import StudioNavigation from '../../components/StudioNavigation.vue'

describe('StudioNavigation account profile', () => {
  it('shows the display name and private avatar in desktop and mobile navigation', () => {
    for (const drawer of [false, true]) {
      const wrapper = mount(StudioNavigation, { props: {
        active: 'overview', locale: 'zh-CN', name: '小健', picture: 'https://auth.example.com/avatar.webp', drawer,
      } })
      const image = wrapper.get('[data-testid="studio-avatar"]')
      expect(image.attributes('src')).toBe('https://auth.example.com/avatar.webp')
      expect(image.attributes('referrerpolicy')).toBe('no-referrer')
      expect(image.attributes('alt')).toBe('')
      expect(wrapper.get('[data-testid="studio-account-name"]').text()).toBe('小健')
      wrapper.unmount()
    }
  })

  it('keeps a fixed avatar placeholder when missing, unsafe or failed', async () => {
    for (const picture of ['', 'javascript:alert(1)', 'data:image/svg+xml,<svg/>', 'https://user:password@auth.example.com/a']) {
      const wrapper = mount(StudioNavigation, { props: { active: 'overview', locale: 'en', name: 'Editor', picture } })
      expect(wrapper.find('[data-testid="studio-avatar"]').exists()).toBe(false)
      expect(wrapper.find('[data-testid="studio-avatar-fallback"]').exists()).toBe(true)
      wrapper.unmount()
    }
    const wrapper = mount(StudioNavigation, { props: {
      active: 'overview', locale: 'en', name: 'Editor', picture: 'https://auth.example.com/a.webp', collapsed: true,
    } })
    await wrapper.get('[data-testid="studio-avatar"]').trigger('error')
    expect(wrapper.find('[data-testid="studio-avatar-fallback"]').exists()).toBe(true)
    await wrapper.setProps({ picture: 'https://auth.example.com/b.webp' })
    expect(wrapper.get('[data-testid="studio-avatar"]').attributes('src')).toBe('https://auth.example.com/b.webp')
    wrapper.unmount()
  })
})
