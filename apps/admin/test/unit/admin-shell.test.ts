import { mountSuspended } from '@nuxt/test-utils/runtime'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import App from '../../app.vue'
import AdminPage from '../../pages/index.vue'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('管理端页面', () => {
  it('渲染内容工作台和安全的公开站链接', async () => {
    const wrapper = await mountSuspended(AdminPage)

    expect(wrapper.find('[data-testid="admin-workspace"]').exists()).toBe(true)
    expect(wrapper.get('h1').text()).toMatch(/内容工作台|Content workspace/)
    expect(wrapper.get('[data-testid="snapshot-preview"]').text()).toBe('{}')
    expect(wrapper.get('[data-testid="snapshot-import"]').attributes('aria-label')).toBeTruthy()
    expect(wrapper.get('[data-testid="snapshot-export"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="snapshot-file-input"]').attributes('aria-label')).toBeTruthy()
    expect(wrapper.get('[data-testid="snapshot-file-input"]').attributes('tabindex')).toBe('-1')
    expect(wrapper.get('[data-testid="snapshot-validation"]').text()).toMatch(/错误|issue/i)
    expect(wrapper.get('a[href="https://yujian.me"]').attributes('rel')).toBe('noopener noreferrer')
  })

  it('从本地文件导入快照', async () => {
    const wrapper = await mountSuspended(AdminPage)
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    const contents = JSON.stringify(fixtureData)
    const file = new File([contents], 'draft.json', { type: 'application/json' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')

    expect((wrapper.get('.json-editor').element as HTMLTextAreaElement).value).toContain('rel_fixture_20260829')
    expect(wrapper.get('[data-testid="snapshot-validation"]').text()).toMatch(/有效|valid/i)
  })

  it('导出合法快照并释放临时 URL', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:snapshot')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = await mountSuspended(AdminPage)

    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await wrapper.get('[data-testid="snapshot-export"]').trigger('click')

    expect(createObjectURL).toHaveBeenCalledOnce()
    expect((createObjectURL.mock.calls[0]?.[0] as Blob).type).toBe('application/json')
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:snapshot')
  })

  it('允许在当前标签页切换管理界面语言', async () => {
    const wrapper = await mountSuspended(AdminPage)
    const localeButton = wrapper.get('.rail-locale')
    const initialTitle = wrapper.get('h1').text()

    await localeButton.trigger('click')

    expect(wrapper.get('[data-testid="snapshot-validation"]').text()).toMatch(/错误|issue/i)
    expect(wrapper.get('h1').text()).toMatch(/内容工作台|Content workspace/)
    expect(wrapper.get('h1').text()).not.toBe(initialTitle)
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
