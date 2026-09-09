import { mountSuspended } from '@nuxt/test-utils/runtime'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import App from '../../app.vue'
import AdminPage from '../../pages/index.vue'

/** 创建可由测试精确控制完成时机的 Promise。 */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  localStorage.clear()
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
    vi.useFakeTimers()
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    const contents = JSON.stringify(fixtureData)
    const file = new File([contents], 'draft.json', { type: 'application/json' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)

    expect((wrapper.get('.json-editor').element as HTMLTextAreaElement).value).toContain('rel_fixture_20260829')
    expect(wrapper.get('[data-testid="snapshot-validation"]').text()).toMatch(/有效|valid/i)
  })

  it('导入期间禁用重复导入、编辑和保存', async () => {
    const wrapper = await mountSuspended(AdminPage)
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    const pendingContents = deferred<string>()
    const file = new File(['pending'], 'draft.json', { type: 'application/json' })
    vi.spyOn(file, 'text').mockReturnValue(pendingContents.promise)
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')

    expect(wrapper.get('[data-testid="snapshot-import"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.json-editor').attributes('disabled')).toBeDefined()
    expect(wrapper.get('.button--primary').attributes('disabled')).toBeDefined()

    pendingContents.resolve(JSON.stringify(fixtureData))
    await flushPromises()

    expect(wrapper.get('[data-testid="snapshot-import"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('.json-editor').attributes('disabled')).toBeUndefined()
  })

  it('导出合法快照并释放临时 URL', async () => {
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:snapshot')
    const revokeObjectURL = vi.spyOn(URL, 'revokeObjectURL').mockImplementation(() => {})
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()

    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="snapshot-export"]').trigger('click')

    expect(createObjectURL).toHaveBeenCalledOnce()
    expect((createObjectURL.mock.calls[0]?.[0] as Blob).type).toBe('application/json')
    expect(click).toHaveBeenCalledOnce()
    expect(revokeObjectURL).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(0)

    expect(revokeObjectURL).toHaveBeenCalledWith('blob:snapshot')
  })

  it('按当前界面语言显示导入错误', async () => {
    const wrapper = await mountSuspended(AdminPage)
    if (wrapper.get('h1').text() !== 'Content workspace') {
      await wrapper.get('.rail-locale').trigger('click')
    }
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    const file = new File(['{}'], 'draft.txt', { type: 'text/plain' })
    Object.defineProperty(input.element, 'files', { configurable: true, value: [file] })

    await input.trigger('change')
    await flushPromises()

    expect(wrapper.get('.notice').text()).toContain('Choose a JSON file')
  })

  it('允许在当前标签页切换管理界面语言', async () => {
    const wrapper = await mountSuspended(AdminPage)
    const localeButton = wrapper.get('.rail-locale')

    if (wrapper.get('h1').text() !== 'Content workspace') {
      await localeButton.trigger('click')
    }
    await flushPromises()

    expect(wrapper.get('[data-testid="snapshot-validation"]').text()).toMatch(/错误|issue/i)
    expect(wrapper.get('h1').text()).toBe('Content workspace')
    expect(document.documentElement.lang).toBe('en')
    expect(wrapper.get('.json-editor').attributes('aria-label')).toBe('JSON snapshot editor')
  })

  it('防抖并本地化字段提示和预览', async () => {
    const wrapper = await mountSuspended(AdminPage)
    if (wrapper.get('h1').text() !== 'Content workspace') {
      await wrapper.get('.rail-locale').trigger('click')
    }
    vi.useFakeTimers()

    await wrapper.get('.json-editor').setValue('{')

    expect(wrapper.find('.field-error').exists()).toBe(false)
    expect(wrapper.get('[data-testid="snapshot-preview"]').text()).toBe('{}')

    await vi.advanceTimersByTimeAsync(1000)

    expect(wrapper.get('.field-error').text()).toBe('Invalid JSON')
    expect(wrapper.get('[data-testid="snapshot-preview"]').text()).toBe('Invalid JSON')
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
