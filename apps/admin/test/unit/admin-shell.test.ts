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
  it.each([
    ['draft', false, 'Submit review'],
    ['in_review', false, 'Approve'],
    ['in_review', true, 'Publish'],
    ['published', true, 'Rollback'],
  ])('现场未应用输入阻止 %s / %s 对应的 %s 操作', async (status, reviewApproved, action) => {
    localStorage.setItem('yujian:admin-locale', 'en')
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ id: 'ver_gate', status, reviewApproved, revision: 1, snapshot: fixtureData, checksum: 'sha256:test' })))
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.connection-grid input[type="text"]').setValue('ver_gate')
    await wrapper.get('[data-testid="version-load"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    const button = wrapper.findAll('button').find(candidate => candidate.text() === action)!
    expect(button.attributes('disabled')).toBeUndefined()
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="event-title-en"]').setValue('Unapplied event')
    expect(button.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="event-close"]').trigger('click')
    await wrapper.get('[data-testid="event-discard"]').trigger('click')
    expect(button.attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('现场独立输入覆盖全局门禁与离开保护，应用仅改 events 并使其他三表单过期', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="event-title-zh"]').setValue('应用现场')
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    expect(wrapper.find('[data-testid="preview-form-dirty"]').exists()).toBe(true)
    for (const id of ['snapshot-save', 'snapshot-export']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    const leaving = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(leaving)
    expect(leaving.defaultPrevented).toBe(true)
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('影像待处理')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('首页待处理')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('音乐待处理')
    await wrapper.get('[data-testid="event-apply"]').trigger('click')
    for (const id of ['video-apply', 'homepage-apply', 'music-apply']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    for (const [id, value] of [['video-title-zh', '影像待处理'], ['homepage-headline-zh', '首页待处理'], ['release-title-zh', '音乐待处理']]) expect(wrapper.get(`[data-testid="${id}"]`).element).toHaveProperty('value', value)
    const value = JSON.parse((wrapper.get('.json-editor').element as HTMLTextAreaElement).value)
    expect(value).toEqual({ ...fixtureData, events: [{ ...fixtureData.events[0], title: { ...fixtureData.events[0]!.title, 'zh-CN': '应用现场' } }, fixtureData.events[1]] })
    wrapper.unmount()
  })

  it('保存成功刷新四个已打开工作台基线，409 与忙碌期间保护影像和现场', async () => {
    const loaded = { id: 'ver_video', status: 'draft', revision: 4, snapshot: fixtureData, checksum: 'sha256:test' }
    const saving = deferred<Response>()
    const fetcher = vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify(loaded)))
      .mockResolvedValueOnce(new Response(JSON.stringify({ code: 'revision_conflict', message: 'Conflict', requestId: 'req_video' }), { status: 409 }))
      .mockReturnValueOnce(saving.promise)
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.connection-grid input[type="text"]').setValue('ver_video')
    await wrapper.get('[data-testid="version-load"]').trigger('click')
    await flushPromises()
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="video-title-en"]').setValue('Saved video')
    await wrapper.get('[data-testid="video-apply"]').trigger('click')
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('.notice--error').text()).toContain('req_video')
    expect(wrapper.get('[data-testid="video-title-en"]').element).toHaveProperty('value', 'Saved video')
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-fields"]').attributes('disabled')).toBeDefined()
    for (const id of ['video-add', 'video-open', 'video-close', 'video-apply', 'event-add', 'event-open', 'event-close', 'event-apply']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    const saved = JSON.parse(String(fetcher.mock.calls[2]![1]!.body)).snapshot
    expect(new Headers(fetcher.mock.calls[2]![1]!.headers).get('If-Match')).toBe('"4"')
    saving.resolve(new Response(JSON.stringify({ ...loaded, revision: 5, snapshot: saved })))
    await flushPromises()
    await wrapper.get('[data-testid="video-title-en"]').setValue('Next video')
    await wrapper.get('[data-testid="homepage-headline-en"]').setValue('Next homepage')
    await wrapper.get('[data-testid="release-title-en"]').setValue('Next music')
    await wrapper.get('[data-testid="event-title-en"]').setValue('Next event')
    for (const id of ['video-apply', 'homepage-apply', 'music-apply', 'event-apply']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('替换请求取消四个工作台待确认意图，失败后保留全部输入', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('保留影像')
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="event-title-zh"]').setValue('保留现场')
    await wrapper.get('[data-testid="event-close"]').trigger('click')
    await wrapper.get('[data-testid="video-close"]').trigger('click')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('保留首页')
    await wrapper.get('[data-testid="homepage-close"]').trigger('click')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('保留音乐')
    await wrapper.get('[data-testid="music-close"]').trigger('click')
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['{}'], 'bad.txt')] })
    await input.trigger('change')
    await flushPromises()
    for (const name of ['video', 'homepage', 'music', 'event']) expect(wrapper.find(`[data-testid="${name}-discard-prompt"]`).exists()).toBe(false)
    await wrapper.get('[data-testid="snapshot-replace-discard"]').trigger('click')
    await flushPromises()
    for (const [id, value] of [['video-title-zh', '保留影像'], ['homepage-headline-zh', '保留首页'], ['release-title-zh', '保留音乐'], ['event-title-zh', '保留现场']]) expect(wrapper.get(`[data-testid="${id}"]`).element).toHaveProperty('value', value)
    wrapper.unmount()
  })
  it('影像输入加入全局门禁，其他工作台应用使影像过期，失败替换不丢输入', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('影像待处理')
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    expect(wrapper.find('[data-testid="preview-form-dirty"]').exists()).toBe(true)
    for (const id of ['snapshot-save', 'snapshot-export']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    const leaving = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(leaving)
    expect(leaving.defaultPrevented).toBe(true)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-en"]').setValue('Applied homepage')
    await wrapper.get('[data-testid="homepage-apply"]').trigger('click')
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '影像待处理')
    expect(wrapper.get('[data-testid="video-apply"]').attributes('disabled')).toBeDefined()
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['{}'], 'bad.txt')] })
    await input.trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="snapshot-replace-discard"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '影像待处理')
    wrapper.unmount()
  })

  it('应用影像只更新 videos 并保留首页与音乐的临时输入', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('首页待处理')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('音乐待处理')
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('应用影像')
    await wrapper.get('[data-testid="video-apply"]').trigger('click')
    for (const id of ['homepage-apply', 'music-apply', 'snapshot-save']) expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="homepage-headline-zh"]').element).toHaveProperty('value', '首页待处理')
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '音乐待处理')
    const value = JSON.parse((wrapper.get('.json-editor').element as HTMLTextAreaElement).value)
    expect(value.videos[0].title['zh-CN']).toBe('应用影像')
    expect(value.homepage).toEqual(fixtureData.homepage)
    expect(value.releases).toEqual(fixtureData.releases)
    wrapper.unmount()
  })
  it('从唯一 JSON 源更新真实预览，编辑及改回原文也销毁旧文档', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    const text = JSON.stringify(fixtureData)
    await wrapper.get('.json-editor').setValue(text)
    await vi.advanceTimersByTimeAsync(250)
    await wrapper.get('[data-testid="preview-open"]').trigger('click')
    const first = wrapper.get('iframe').element
    await wrapper.get('.json-editor').setValue('{')
    expect(wrapper.find('iframe').exists()).toBe(false)
    await wrapper.get('.json-editor').setValue(text)
    expect(wrapper.get('iframe').element).not.toBe(first)
    const restored = wrapper.get('iframe').element
    const changed = structuredClone(fixtureData)
    changed.site.brand.en = 'New draft'
    await wrapper.get('.json-editor').setValue(JSON.stringify(changed))
    expect(wrapper.find('iframe').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(250)
    expect(wrapper.get('iframe').element).not.toBe(restored)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('未应用')
    expect(wrapper.find('[data-testid="preview-form-dirty"]').exists()).toBe(true)
    expect(wrapper.find('iframe').exists()).toBe(true)
    wrapper.unmount()
  })
  it('首页应用不会覆盖音乐临时输入，并使音乐基线过期', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('待处理音乐')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('首页先应用')
    await wrapper.get('[data-testid="homepage-apply"]').trigger('click')
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '待处理音乐')
    expect(wrapper.get('[data-testid="music-apply"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="snapshot-export"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="music-close"]').trigger('click')
    await wrapper.get('[data-testid="music-discard"]').trigger('click')
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeUndefined()
    const snapshot = JSON.parse((wrapper.get('.json-editor').element as HTMLTextAreaElement).value)
    expect(snapshot.heroSlides[0].headline['zh-CN']).toBe('首页先应用')
    expect(snapshot.releases[0].title).toEqual(fixtureData.releases[0]!.title)
    wrapper.unmount()
  })

  it('首页未应用草稿禁用保存与导出，失败替换保留输入', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('首页未应用')
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="snapshot-export"]').attributes('disabled')).toBeDefined()
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['{}'], 'bad.txt')] })
    await input.trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="snapshot-replace-discard"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="homepage-headline-zh"]').element).toHaveProperty('value', '首页未应用')
    wrapper.unmount()
  })

  it('音乐与首页保留各自输入，任一应用使另一表单过期', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('待处理首页')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('音乐先应用')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    expect(wrapper.get('[data-testid="homepage-headline-zh"]').element).toHaveProperty('value', '待处理首页')
    expect(wrapper.get('[data-testid="homepage-apply"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="homepage-close"]').trigger('click')
    await wrapper.get('[data-testid="homepage-discard"]').trigger('click')
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeUndefined()
    const snapshot = JSON.parse((wrapper.get('.json-editor').element as HTMLTextAreaElement).value)
    expect(snapshot.releases[0].title['zh-CN']).toBe('音乐先应用')
    expect(snapshot.heroSlides[0].headline).toEqual(fixtureData.heroSlides[0]!.headline)
    wrapper.unmount()
  })

  it('保护未保存快照的导入，并为未应用表单注册离开提示', async () => {
    const wrapper = await mountSuspended(AdminPage)
    vi.useFakeTimers()
    await wrapper.get('.json-editor').setValue(JSON.stringify(fixtureData))
    await vi.advanceTimersByTimeAsync(1000)
    expect(wrapper.get('[data-testid="snapshot-unsaved"]').text()).toMatch(/未保存|Unsaved/)
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('未应用')
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeDefined()
    const leaving = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(leaving)
    expect(leaving.defaultPrevented).toBe(true)
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File(['{}'], 'replace.json')] })
    await input.trigger('change')
    await flushPromises()
    expect(wrapper.find('[data-testid="snapshot-replace-prompt"]').exists()).toBe(true)
    expect((wrapper.get('.json-editor').element as HTMLTextAreaElement).value).toContain('release_01')
    await wrapper.get('[data-testid="snapshot-replace-keep"]').trigger('click')
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '未应用')
    await input.trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="snapshot-replace-discard"]').trigger('click')
    await flushPromises()
    expect((wrapper.get('.json-editor').element as HTMLTextAreaElement).value).toBe('{}')
    expect(wrapper.find('[data-testid="release-title-zh"]').exists()).toBe(false)
    wrapper.unmount()
  })

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
    expect(wrapper.find('[data-testid="asset-workbench"]').exists()).toBe(true)
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
