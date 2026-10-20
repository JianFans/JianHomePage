import { mountSuspended } from '@nuxt/test-utils/runtime'
import { flushPromises } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import AdminPage from '../../pages/index.vue'
import type { StudioWorkspace } from '../../composables/useStudioWorkspace'

/** 模拟真实 HTTP 边界，不替代页面的表单、事务和版本状态。 */
function backend(options: { authenticated?: boolean; status?: string; reviewApproved?: boolean; roles?: string[]; failSave?: boolean; failList?: boolean; empty?: boolean; paginated?: boolean; logout?: Promise<Response> } = {}) {
  const records = new Map([
    ['ver_14cea400-a42f-4560-9770-3de88cae9d5a', { id: 'ver_14cea400-a42f-4560-9770-3de88cae9d5a', name: '工作草稿', status: options.status || 'draft', reviewApproved: options.reviewApproved, revision: 4, snapshot: structuredClone(fixture), checksum: 'sha256:test' }],
    ['ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', { id: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', name: '之前版本', status: 'archived', revision: 2, snapshot: structuredClone(fixture), checksum: 'sha256:history' }],
  ])
  return vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init) => {
    const url = new URL(String(input), 'http://localhost')
    const path = url.pathname
    const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status })
    if (path.endsWith('/auth/session')) return json({ authenticated: options.authenticated !== false, loginAvailable: true,
      principal: { subject: 'editor_1', roles: options.roles || ['admin'] }, csrfToken: 'csrf-test', expiresAt: new Date(Date.now() + 3600000).toISOString() })
    if (path.endsWith('/auth/logout')) return options.logout || new Response(null, { status: 204 })
    if (path.endsWith('/versions')) {
      if (init?.method === 'POST') {
        const body = JSON.parse(String(init.body))
        return json({ ...records.get('ver_14cea400-a42f-4560-9770-3de88cae9d5a'), id: 'ver_e26e5049-a6cb-42b5-b448-9d927c4d0040', snapshot: body.snapshot })
      }
      if (options.failList) return json({ code: 'unavailable', message: 'Unavailable' }, 503)
      if (options.paginated) return json(url.searchParams.has('cursor')
        ? { items: [records.get('ver_14cea400-a42f-4560-9770-3de88cae9d5a')] }
        : { items: [records.get('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')], nextCursor: 'older-page' })
      return json({ items: options.empty ? [] : [...records.values()].map(record => ({ id: record.id, name: record.name, status: record.status, reviewApproved: record.reviewApproved, revision: record.revision, checksum: record.checksum })) })
    }
    if (path.endsWith('/clone')) return json({ ...records.get('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921'), id: 'ver_9cb0468e-f117-476b-be75-eed979e0c719', name: '复制版本', sourceVersionId: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', status: 'draft', revision: 1 })
    const id = path.split('/').at(-1)!
    const record = records.get(id)
    if (!record) return json({ code: 'not_found', message: 'Not found' }, 404)
    if (init?.method === 'PUT') {
      if (options.failSave) return json({ code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_f9f9abc2-e321-40c6-a1f8-be24bb71186b' }, 409)
      const body = JSON.parse(String(init.body))
      return json({ ...record, revision: record.revision + 1, snapshot: body.snapshot })
    }
    return json(record)
  })
}

/** 等待真实恢复会话与版本读取，再推进界面分析的防抖。 */
async function mountStudio(options: Parameters<typeof backend>[0] = {}) {
  localStorage.setItem('yujian:admin-locale', 'en')
  const fetcher = backend(options)
  const wrapper = await mountSuspended(AdminPage, { attachTo: document.body })
  await flushPromises()
  await vi.waitFor(() => expect(wrapper.find('[data-testid="studio-loading"]').exists()).toBe(false))
  await new Promise(resolve => setTimeout(resolve, 300))
  await flushPromises()
  return { wrapper, fetcher }
}

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); vi.useRealTimers(); localStorage.clear() })

it('shows approved review consistently and prevents reopening review decisions', async () => {
  const { wrapper } = await mountStudio({ status: 'in_review', reviewApproved: true })
  try {
    expect(wrapper.get('.studio-meta').text()).toContain('Approved')
    expect(wrapper.get('[data-testid="version-ver_14cea400-a42f-4560-9770-3de88cae9d5a"]').text()).toContain('Approved')
    expect(wrapper.get('.studio-version-select').text()).toContain('Approved')
    expect(wrapper.get('.studio-history-list').text()).toContain('Approved')
    await wrapper.get('[data-testid="nav-history"]').trigger('click')
    expect(wrapper.get('.studio-history-list').text()).toContain('Approved')
    const reviewButton = wrapper.findAll('.studio-workflow-actions button').find(button => button.text().includes('Approve'))!
    expect(reviewButton.attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('.studio-workflow-actions button').find(button => button.text() === 'Publish')!.attributes('disabled')).toBeUndefined()
    await wrapper.get('button[aria-label="切换中文"]').trigger('click')
    expect(wrapper.get('.studio-meta').text()).toContain('审核通过')
  } finally { wrapper.unmount() }
})

it('opens a usable first draft without importing test content or writing to the server', async () => {
  const { wrapper, fetcher } = await mountStudio({ empty: true })
  try {
    const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
    expect(studio.workspace.editorAnalysis.snapshot?.site.brand['zh-CN']).toBe('遇健我')
    expect(studio.workspace.editorAnalysis.snapshot?.assets).toEqual([])
    expect(studio.workspace.editorAnalysis.snapshot?.releases).toEqual([])
    expect(studio.workspace.editorAnalysis.snapshot?.homepage.sections.map(section => section.type)).toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
    expect(studio.workspace.dirty).toBe(true)
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
    await wrapper.get('[data-testid="nav-homepage"]').trigger('click')
    expect(studio.homepage.draft?.heroSlides).toEqual([])
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(studio.workspace.version?.id).toBe('ver_e26e5049-a6cb-42b5-b448-9d927c4d0040')
  } finally { wrapper.unmount() }
})

it('does not initialize content for a read-only empty account', async () => {
  const { wrapper, fetcher } = await mountStudio({ empty: true, roles: ['reviewer'] })
  try {
    const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
    expect(studio.workspace.editorText).toBe('{}')
    expect(studio.workspace.dirty).toBe(false)
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  } finally { wrapper.unmount() }
})

it('keeps local input when initialization is retried against an empty server', async () => {
  const { wrapper } = await mountStudio({ empty: true })
  try {
    const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
    studio.workspace.editorText = '{"local":"unsaved"}'
    await studio.initialize()
    expect(studio.workspace.editorText).toBe('{"local":"unsaved"}')
  } finally { wrapper.unmount() }
})

it('keeps unsaved initial content when another editor creates a server version', async () => {
  const options = { empty: true }
  const { wrapper, fetcher } = await mountStudio(options)
  try {
    const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
    studio.workspace.editorText = '{"local":"unsaved"}'
    options.empty = false
    fetcher.mockClear()
    await studio.initialize()
    expect(studio.workspace.versions).toHaveLength(2)
    expect(studio.workspace.editorText).toBe('{"local":"unsaved"}')
    expect(studio.workspace.version).toBeNull()
    expect(fetcher.mock.calls.some(([url]) => String(url).includes('/versions/ver_'))).toBe(false)
  } finally { wrapper.unmount() }
})

it('does not mistake a failed version listing for a new empty database', async () => {
  const { wrapper, fetcher } = await mountStudio({ failList: true })
  try {
    const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
    expect(studio.workspace.editorText).toBe('{}')
    expect(studio.workspace.dirty).toBe(false)
    expect(studio.workspace.listError).toBe(true)
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(false)
  } finally { wrapper.unmount() }
})

it.each([
  { status: 'archived', roles: ['admin'] },
  { status: 'published', roles: ['admin'] },
  { status: 'in_review', roles: ['admin'] },
  { status: 'draft', roles: ['reviewer'] },
  { status: 'draft', roles: ['publisher'] },
])('allows browsing read-only homepage heroes for $status with $roles', async (options) => {
  const { wrapper, fetcher } = await mountStudio(options)
  try {
    await wrapper.get('[data-testid="nav-homepage"]').trigger('click')
    const next = wrapper.get('[data-testid="homepage-hero-hero_cec07764-0923-4dcc-a57b-a8630ce91f51"]')
    expect(next.element.matches(':disabled')).toBe(false)
    expect(next.element.closest('fieldset[disabled]')).toBeNull()
    await next.trigger('click')
    expect(wrapper.get('[data-testid="homepage-hero-id"]').text()).toBe('hero_cec07764-0923-4dcc-a57b-a8630ce91f51')
    expect(wrapper.get('[data-testid="homepage-headline-zh"]').element).toHaveProperty('value', fixture.heroSlides[1]!.headline['zh-CN'])
    for (const control of wrapper.findAll('[data-testid="homepage-workbench"] input, [data-testid="homepage-workbench"] select, [data-testid="homepage-workbench"] textarea')) {
      expect(control.element.closest('fieldset[disabled]'), control.element.outerHTML).not.toBeNull()
    }
    expect(wrapper.get('[data-testid="homepage-hero-add"]').element.matches(':disabled')).toBe(true)
    expect(wrapper.get('[data-testid="homepage-hero-remove"]').element.matches(':disabled')).toBe(true)
    expect(wrapper.get('[data-testid="snapshot-save"]').element.matches(':disabled')).toBe(true)
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
  } finally {
    wrapper.unmount()
  }
})

it.each(['video', 'event', 'moment'] as const)('allows browsing every read-only %s record', async (module) => {
  const { wrapper } = await mountStudio({ status: 'archived' })
  await wrapper.get(`[data-testid="nav-${module}"]`).trigger('click')
  const records = wrapper.findAll(`[data-testid^="${module}-record-"]`)
  expect(records.length).toBeGreaterThan(1)
  await records[1]!.trigger('click')
  expect(records[1]!.attributes('aria-pressed')).toBe('true')
  expect(wrapper.get(`[data-testid="${module}-fields"]`).attributes('disabled')).toBeDefined()
  wrapper.unmount()
})

it('retains the asset form when switching modules', async () => {
  const { wrapper } = await mountStudio()
  await wrapper.get('[data-testid="nav-assets"]').trigger('click')
  await wrapper.get('[data-testid="asset-source-zh"]').setValue('Local rights source')
  await wrapper.get('[data-testid="nav-overview"]').trigger('click')
  await wrapper.get('[data-testid="nav-assets"]').trigger('click')
  expect(wrapper.get('[data-testid="asset-source-zh"]').element).toHaveProperty('value', 'Local rights source')
  wrapper.unmount()
})

it('selects the newest draft beyond the first summary page', async () => {
  const { wrapper, fetcher } = await mountStudio({ paginated: true })
  expect(wrapper.get('[data-testid="studio-version-name"]').text()).toBe('工作草稿')
  expect(fetcher.mock.calls.some(([input]) => String(input).includes('cursor=older-page'))).toBe(true)
  wrapper.unmount()
})

it('retains selected asset files and inputs across anonymous and authenticated session recovery', async () => {
  const { wrapper, fetcher } = await mountStudio()
  await wrapper.get('[data-testid="nav-assets"]').trigger('click')
  const file = new File(['asset'], 'retained.webp', { type: 'image/webp' })
  const input = wrapper.get('[data-testid="asset-file-input"]')
  Object.defineProperty(input.element, 'files', { value: [file], configurable: true })
  await input.trigger('change')
  await wrapper.get('[data-testid="asset-source-zh"]').setValue('Retained source')
  const page = wrapper.vm as unknown as { studio: { session: { expire: () => void; restore: () => Promise<void> } } }
  page.studio.session.expire()
  await flushPromises()
  expect(wrapper.get('[data-testid="studio-shell"]').isVisible()).toBe(false)
  expect(wrapper.find('[data-testid="studio-login"]').exists()).toBe(true)
  fetcher.mockResolvedValueOnce(new Response(JSON.stringify({ authenticated: false, loginAvailable: true })))
  await page.studio.session.restore()
  await flushPromises()
  expect(wrapper.get('[data-testid="studio-shell"]').isVisible()).toBe(false)
  await page.studio.session.restore()
  await flushPromises()
  expect(wrapper.get('[data-testid="asset-source-zh"]').element).toHaveProperty('value', 'Retained source')
  expect((wrapper.get('[data-testid="asset-file-input"]').element as HTMLInputElement).files?.[0]).toBe(file)
  wrapper.unmount()
})

it('clears the saved baseline after logout', async () => {
  const { wrapper } = await mountStudio()
  await wrapper.get('[data-testid="studio-logout"]').trigger('click')
  await flushPromises()
  expect(wrapper.find('[data-testid="studio-login"]').exists()).toBe(true)
  const event = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(event)
  expect(event.defaultPrevented).toBe(false)
  wrapper.unmount()
})

it.each(['homepage', 'assets'] as const)('locks %s during logout and restores input after failure', async (module) => {
  let finish!: (response: Response) => void
  const logout = new Promise<Response>((resolve) => { finish = resolve })
  const { wrapper, fetcher } = await mountStudio({ logout })
  const { studio } = wrapper.vm as unknown as { studio: StudioWorkspace }
  try {
    await wrapper.get(`[data-testid="nav-${module}"]`).trigger('click')
    const input = wrapper.get(`[data-testid="${module === 'homepage' ? 'homepage-headline-en' : 'asset-source-zh'}"]`)
    await input.setValue('Retain on logout failure')
    await wrapper.get('[data-testid="studio-logout"]').trigger('click')
    await wrapper.get('[data-testid="leave-discard"]').trigger('click')
    await flushPromises()
    expect(studio.workspace.busy).toBe(true)
    expect(wrapper.get('[data-testid="studio-shell"]').attributes('inert')).toBeDefined()
    expect(input.element.matches(':disabled') || input.element.closest('fieldset[disabled]') !== null).toBe(true)
    for (const id of ['snapshot-save', 'version-clone', 'version-ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', 'nav-video']) {
      expect(wrapper.get(`[data-testid="${id}"]`).attributes('disabled')).toBeDefined()
    }
    const calls = fetcher.mock.calls.length
    await studio.request({ kind: 'module', module: 'video' })
    await studio.request({ kind: 'version', id: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921' })
    await studio.request({ kind: 'clone' })
    await studio.save()
    await studio.clone('Forbidden during logout')
    await studio.workspace.saveDraft()
    expect(studio.module).toBe(module)
    expect(studio.pending).toBeNull()
    expect(studio.cloneRequested).toBe(false)
    expect(fetcher.mock.calls).toHaveLength(calls)
    finish(new Response(JSON.stringify({ code: 'unavailable', message: 'Logout failed' }), { status: 500 }))
    await flushPromises()
    expect(studio.workspace.busy).toBe(false)
    expect(wrapper.get('[data-testid="studio-shell"]').attributes('inert')).toBeUndefined()
    expect(input.element).toHaveProperty('value', 'Retain on logout failure')
    expect(input.element.matches(':disabled')).toBe(false)
    expect(wrapper.get('[data-testid="nav-video"]').attributes('disabled')).toBeUndefined()
    expect(wrapper.get('[data-testid="toast-viewport"]').text()).toContain('Sign-out failed')
  } finally {
    finish(new Response(null, { status: 204 }))
    await flushPromises()
    wrapper.unmount()
  }
})

it('closes active dialogs on session expiry while retaining unsaved content', async () => {
  const { wrapper, fetcher } = await mountStudio()
  await wrapper.get('[data-testid="studio-tools"]').trigger('click')
  await wrapper.get('.json-editor').setValue(JSON.stringify({ ...fixture, releaseId: 'rel_35662245-24ad-4ba2-a432-b13bf2e9cc7e' }))
  fetcher.mockResolvedValue(new Response(JSON.stringify({ code: 'unauthorized', message: 'Unauthorized' }), { status: 401 }))
  const page = wrapper.vm as unknown as { studio: { save: () => Promise<boolean> } }
  await page.studio.save()
  await flushPromises()
  expect(wrapper.find('[data-testid="studio-login"]').exists()).toBe(true)
  expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
  const event = new Event('beforeunload', { cancelable: true })
  window.dispatchEvent(event)
  expect(event.defaultPrevented).toBe(true)
  wrapper.unmount()
})

describe('CMS 版本工作区', () => {
  it('无效表单用 Toast 提示，选择其他作品也要明确确认', async () => {
    const { wrapper, fetcher } = await mountStudio()
    await wrapper.get('[data-testid="nav-music"]').trigger('click')
    await wrapper.get('[data-testid="music-release-release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('')
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'PUT')).toBe(false)
    expect(wrapper.find('[data-testid="music-error"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="toast-viewport"]').text()).toContain('Check the fields')
    const invalid = wrapper.get('[data-testid="release-title-zh"]').element
    expect(document.activeElement).toBe(invalid)
    expect(invalid.getAttribute('aria-invalid')).toBe('true')
    expect(document.getElementById(invalid.getAttribute('aria-describedby') || '')?.textContent)
      .toBe('Text is too short · /releases/0/title/zh-CN')
    await wrapper.get('[data-testid="music-release-release_f9462a63-70d9-40b3-a0cc-a792f1b5499e"]').trigger('click')
    expect(wrapper.find('[data-testid="leave-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="leave-keep"]').trigger('click')
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '')
    wrapper.unmount()
  })
  it('匿名只有登录入口，不要求用户填写连接地址或令牌', async () => {
    const { wrapper } = await mountStudio({ authenticated: false })
    expect(wrapper.get('[data-testid="studio-login"]').attributes('href')).toContain('/api/v1/auth/login')
    expect(wrapper.find('[data-testid="studio-shell"]').exists()).toBe(false)
    expect(wrapper.find('input[type="password"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('侧栏真实切换模块，一次只显示一个工作台', async () => {
    const { wrapper } = await mountStudio()
    await wrapper.get('[data-testid="nav-music"]').trigger('click')
    expect(wrapper.find('[data-testid="music-workbench"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="video-workbench"]').exists()).toBe(false)
    await wrapper.get('[data-testid="nav-video"]').trigger('click')
    expect(wrapper.find('[data-testid="music-workbench"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="video-workbench"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('顶部保存应用当前表单并提交，失败仍保留输入与当前版本', async () => {
    const { wrapper, fetcher } = await mountStudio({ failSave: true })
    await wrapper.get('[data-testid="nav-music"]').trigger('click')
    await wrapper.get('[data-testid="music-release-release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('保留标题')
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '保留标题')
    expect(wrapper.get('[data-testid="studio-version-name"]').text()).toContain('工作草稿')
    const put = fetcher.mock.calls.find(([, init]) => init?.method === 'PUT')!
    expect(JSON.parse(String(put[1]?.body)).snapshot.releases[0].title['zh-CN']).toBe('保留标题')
    expect(wrapper.get('[data-testid="toast-viewport"]').text()).toContain('Revision conflict')
    wrapper.unmount()
  })

  it('脏表单切模块提供三种选择，取消保留输入、放弃后只打开目标模块', async () => {
    const { wrapper } = await mountStudio()
    await wrapper.get('[data-testid="nav-video"]').trigger('click')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('未保存影像')
    await wrapper.get('[data-testid="nav-moment"]').trigger('click')
    expect(wrapper.find('[data-testid="leave-dialog"]').exists()).toBe(true)
    await wrapper.get('[data-testid="leave-keep"]').trigger('click')
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '未保存影像')
    await wrapper.get('[data-testid="nav-moment"]').trigger('click')
    await wrapper.get('[data-testid="leave-discard"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="moment-workbench"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="video-workbench"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('历史只读；查看不会发布，基于该版本复制进入独立草稿', async () => {
    const { wrapper, fetcher } = await mountStudio()
    await wrapper.get('[data-testid="version-ver_8bb3a437-061d-4dcd-805e-7d5e6f716921"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="nav-music"]').trigger('click')
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="music-new"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="music-release-release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff"]').trigger('click')
    expect(wrapper.find('[data-testid="release-title-zh"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="music-fields"]').attributes('disabled')).toBeDefined()
    expect(fetcher.mock.calls.some(([url]) => String(url).includes('/publish'))).toBe(false)
    await wrapper.get('[data-testid="version-clone"]').trigger('click')
    await wrapper.get('[data-testid="clone-name"]').setValue('复制版本')
    await wrapper.get('[data-testid="clone-confirm"]').element.closest('form')!.dispatchEvent(new Event('submit', { cancelable: true, bubbles: true }))
    await flushPromises()
    expect(wrapper.get('[data-testid="studio-version-name"]').text()).toContain('复制版本')
    expect(wrapper.get('[data-testid="music-new"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('只读角色不能编辑；仅具备对应角色时才开放审核或发布', async () => {
    const { wrapper } = await mountStudio({ roles: ['reviewer'] })
    expect(wrapper.get('[data-testid="snapshot-save"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="version-clone"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('高级工具可以导入第一份快照，等待异步文件读取再保存', async () => {
    const { wrapper, fetcher } = await mountStudio({ empty: true })
    await wrapper.get('[data-testid="studio-tools"]').trigger('click')
    const input = wrapper.get('[data-testid="snapshot-file-input"]')
    Object.defineProperty(input.element, 'files', { configurable: true, value: [new File([JSON.stringify(fixture)], 'snapshot.json', { type: 'application/json' })] })
    await input.trigger('change')
    await flushPromises()
    await wrapper.get('[data-testid="leave-discard"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="dialog-close"]').trigger('click')
    await wrapper.get('[data-testid="snapshot-save"]').trigger('click')
    await flushPromises()
    expect(fetcher.mock.calls.some(([, init]) => init?.method === 'POST')).toBe(true)
    const post = fetcher.mock.calls.find(([, init]) => init?.method === 'POST')!
    expect(JSON.parse(String(post[1]?.body)).snapshot.releaseId).toBe(fixture.releaseId)
    wrapper.unmount()
  })
})
