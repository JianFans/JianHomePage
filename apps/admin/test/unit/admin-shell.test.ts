import { mountSuspended } from '@nuxt/test-utils/runtime'
import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import App from '../../app.vue'
import AdminPage from '../../pages/index.vue'

describe('管理端页面', () => {
  it('渲染内容工作台和安全的公开站链接', async () => {
    const wrapper = await mountSuspended(AdminPage)

    expect(wrapper.find('[data-testid="admin-workspace"]').exists()).toBe(true)
    expect(wrapper.get('h1').text()).toMatch(/内容工作台|Content workspace/)
    expect(wrapper.get('[data-testid="snapshot-preview"]').text()).toBe('{}')
    expect(wrapper.get('a[href="https://yujian.me"]').attributes('rel')).toBe('noopener noreferrer')
  })

  it('允许在当前标签页切换管理界面语言', async () => {
    const wrapper = await mountSuspended(AdminPage)
    const localeButton = wrapper.get('.rail-locale')
    const initialTitle = wrapper.get('h1').text()

    await localeButton.trigger('click')

    expect(wrapper.get('h1').text()).not.toBe(initialTitle)
    expect(wrapper.get('h1').text()).toMatch(/内容工作台|Content workspace/)
  })

  it('应用壳提供 Nuxt 页面挂载点', () => {
    const wrapper = mount(App, {
      global: {
        stubs: { NuxtPage: { template: '<main data-testid="nuxt-page" />' } },
      },
    })

    expect(wrapper.find('[data-testid="nuxt-page"]').exists()).toBe(true)
  })
})
