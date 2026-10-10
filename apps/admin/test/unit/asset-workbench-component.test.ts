import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import AssetWorkbench from '../../components/AssetWorkbench.vue'
import type { AdminAsset } from '../../utils/admin-api'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

afterEach(/** 还原全局 fetch 和 spy，防止跨测试共享网络响应或样式读取 mock。 */ () => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('素材工作台组件', () => {
  it('保存快照期间阻止素材插入', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ items: [asset('asset_new')] })))
    const wrapper = mountWorkbench()
    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-testid="asset-alt-zh"]').setValue('封面')
    expect(wrapper.get('[data-testid="asset-insert"]').attributes('disabled')).toBeUndefined()
    await wrapper.setProps({ snapshotLocked: true })
    expect(wrapper.get('[data-testid="asset-insert"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="asset-insert"]').trigger('click')
    expect(wrapper.emitted('update:editorText')).toBeUndefined()
    wrapper.unmount()
  })

  it('共享样式作用于子组件内部的按钮和标题', /** 挂载真实共享样式并读取 computed style，覆盖触控尺寸、禁用反馈和标题布局。 */ () => {
    const stylesheet = document.createElement('style')
    stylesheet.textContent = readFileSync(resolve(process.cwd(), 'assets/css/main.css'), 'utf8')
    document.head.append(stylesheet)
    const wrapper = mount(AssetWorkbench, {
      attachTo: document.body,
      props: { locale: 'zh-CN', editorText: '{}', apiBaseUrl: 'https://api.yujian.me', token: '' },
    })
    try {
      const upload = getComputedStyle(wrapper.get('[data-testid="asset-upload"]').element)
      const refresh = getComputedStyle(wrapper.get('[data-testid="asset-refresh"]').element)
      expect(upload.minHeight).toBe('44px')
      expect(refresh.height).toBe('44px')
      expect(refresh.width).toBe('44px')
      expect(upload.cursor).toBe('not-allowed')
      expect(refresh.cursor).toBe('not-allowed')
      expect(Number(upload.opacity)).toBeLessThan(1)
      expect(Number(refresh.opacity)).toBeLessThan(1)
      expect(getComputedStyle(wrapper.get('.panel-heading').element).display).toBe('flex')
      expect(getComputedStyle(wrapper.get('#asset-workbench-title').element).fontSize).toBe('16px')
    } finally {
      wrapper.unmount()
      stylesheet.remove()
    }
  })

  it('切换 API 后同 ID 素材不继承旧连接的替代文本', /** 两个服务端返回同一 ID 时，旧连接的替代文本不能使新素材绕过插入门禁。 */ async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ items: [asset('asset_shared')] })))
    const wrapper = mountWorkbench()
    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-asset-id="asset_shared"] [data-testid="asset-alt-zh"]').setValue('旧连接的替代文本')
    await wrapper.setProps({ apiBaseUrl: 'https://api-b.example' })
    expect(wrapper.find('[data-asset-id="asset_shared"]').exists()).toBe(false)
    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()
    const card = wrapper.get('[data-asset-id="asset_shared"]')
    expect((card.get('[data-testid="asset-alt-zh"]').element as HTMLInputElement).value).toBe('')
    expect(card.get('[data-testid="asset-insert"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it.each([
    ['/media/assets/asset_local/source.webp', 'http://127.0.0.1:8080/media/assets/asset_local/source.webp'],
    ['https://media.yujian.me/assets/asset_local/source.webp', 'https://media.yujian.me/assets/asset_local/source.webp'],
  ])('仅为图片预览解析地址 %s，快照保留原始地址', /** 相对地址仅在展示时基于 API origin 解析，插入快照仍使用服务端原始地址。 */ async (src, preview) => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ items: [{ ...asset('asset_local'), src }] })))
    const wrapper = mountWorkbench()
    await wrapper.setProps({ apiBaseUrl: 'http://127.0.0.1:8080' })
    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()
    const card = wrapper.get('[data-asset-id="asset_local"]')
    expect(card.get('img').attributes('src')).toBe(preview)
    await card.get('[data-testid="asset-alt-zh"]').setValue('本地封面')
    await card.get('[data-testid="asset-insert"]').trigger('click')
    const snapshot = JSON.parse(String(wrapper.emitted('update:editorText')?.[0]?.[0]))
    expect(snapshot.assets.find((item: { id: string }) => item.id === 'asset_local').src).toBe(src)
    wrapper.unmount()
  })

  it('展示双语字段和可访问工具按钮', /** 切换语言后可访问名称同步更新，上传状态保留 polite 播报且文件选择限制 MIME。 */ async () => {
    const wrapper = mountWorkbench()

    expect(wrapper.get('[data-testid="asset-workbench"]').attributes('aria-labelledby')).toBe('asset-workbench-title')
    expect(wrapper.get('[data-testid="asset-refresh"]').attributes('aria-label')).toBeTruthy()
    expect(wrapper.get('[data-testid="asset-file-input"]').attributes('accept')).toContain('image/webp')
    expect(wrapper.get('[data-testid="asset-stage"]').attributes('aria-live')).toBe('polite')
    expect(wrapper.get('[data-testid="asset-status-filter"]').attributes('aria-label')).toBe('状态')
    expect(wrapper.get('[data-testid="asset-kind-filter"]').attributes('aria-label')).toBe('类型')

    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('#asset-workbench-title').text()).toContain('Assets')
    expect(wrapper.get('[data-testid="asset-source-zh"]').attributes('aria-label')).toContain('Chinese')
    expect(wrapper.get('[data-testid="asset-status-filter"]').attributes('aria-label')).toBe('Status')
    expect(wrapper.get('[data-testid="asset-kind-filter"]').attributes('aria-label')).toBe('Type')
  })

  it('使用独立文案报告素材列表加载失败', /** 模拟列表请求离线，确认界面不会误报为创建上传失败。 */ async () => {
    vi.stubGlobal('fetch', vi.fn(async () => {
      throw new Error('offline')
    }))
    const wrapper = mountWorkbench()

    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-testid="asset-list-error"]').text()).toBe('无法加载素材')
    expect(wrapper.text()).not.toContain('无法创建上传')
  })

  it('按 ready、重复 ID 和替代文本状态控制快照插入', /** 仅允许具备替代文本且 ID 未占用的 ready 素材插入，并校验生成的 canonical 快照。 */ async () => {
    const existingId = fixtureData.assets[0]!.id
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({
      items: [
        asset('asset_new', 'ready'),
        asset('asset_pending', 'pending'),
        asset(existingId, 'ready'),
      ],
    })))
    const wrapper = mountWorkbench()

    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()

    const ready = wrapper.get('[data-asset-id="asset_new"]')
    const pending = wrapper.get('[data-asset-id="asset_pending"]')
    const duplicate = wrapper.get(`[data-asset-id="${existingId}"]`)
    expect(ready.get('[data-testid="asset-insert"]').attributes('disabled')).toBeDefined()
    expect(pending.get('[data-testid="asset-insert"]').attributes('disabled')).toBeDefined()
    expect(duplicate.get('[data-testid="asset-insert"]').attributes('disabled')).toBeDefined()

    await ready.get('[data-testid="asset-alt-zh"]').setValue('新封面')
    expect(ready.get('[data-testid="asset-insert"]').attributes('disabled')).toBeUndefined()
    await ready.get('[data-testid="asset-insert"]').trigger('click')

    const emitted = wrapper.emitted('update:editorText')
    const text = emitted?.[0]?.[0]
    expect(typeof text).toBe('string')
    expect(analyzeSnapshotText(String(text)).issues).toHaveLength(0)
    expect(String(text)).toContain('asset_new')
  })

  it('只在服务端返回游标时显示加载更多并追加素材', /** 两页响应验证游标传递、列表追加，以及末页隐藏加载更多入口。 */ async () => {
    const fetcher = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ items: [asset('asset_a')], nextCursor: 'next-page' }))
      .mockResolvedValueOnce(jsonResponse({ items: [asset('asset_b')] }))
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mountWorkbench()

    await wrapper.get('[data-testid="asset-refresh"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="asset-load-more"]').exists()).toBe(true)

    await wrapper.get('[data-testid="asset-load-more"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[data-testid="asset-load-more"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-testid="asset-card"]')).toHaveLength(2)
    expect(String(fetcher.mock.calls[1]?.[0])).toContain('cursor=next-page')
  })

  it('上传期间锁定文件和文本输入，完成后恢复编辑并保留替代文本', /** 用挂起的创建请求检查上传表单锁定，释放后验证输入恢复及替代文本转移到素材卡片。 */ async () => {
    const uploadGate = deferred<Response>()
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/assets/uploads')) return uploadGate.promise
      if (url === 'https://upload.example.test/signed') return new Response(null, { status: 200 })
      if (url.endsWith('/api/v1/assets/asset_uploaded/complete')) return jsonResponse(asset('asset_uploaded'))
      return jsonResponse({ items: [] })
    })
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mountWorkbench()
    const input = wrapper.get('[data-testid="asset-file-input"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['asset'], 'cover.webp', { type: 'image/webp' })],
    })
    await input.trigger('change')
    await wrapper.get('[data-testid="asset-source-zh"]').setValue('官方授权')
    await wrapper.get('[data-testid="asset-alt-upload-zh"]').setValue('封面')

    await wrapper.get('form.asset-upload').trigger('submit')
    await flushPromises()
    expect(wrapper.get('[data-testid="asset-stage"]').text()).toMatch(/创建|creating/i)
    wrapper.findAll('form.asset-upload input').forEach((field) => {
      expect(field.attributes('disabled')).toBeDefined()
    })

    uploadGate.resolve(jsonResponse({
      asset: { ...asset('asset_uploaded', 'pending') },
      uploadUrl: 'https://upload.example.test/signed',
      headers: { 'Content-Type': 'image/webp' },
      expiresAt: '2026-10-09T09:00:00Z',
    }))
    await flushPromises()
    expect(wrapper.get('[data-testid="asset-stage"]').text()).toMatch(/完成|complete/i)
    expect(wrapper.find('[data-asset-id="asset_uploaded"]').exists()).toBe(true)
    expect((wrapper.get('[data-asset-id="asset_uploaded"] [data-testid="asset-alt-zh"]').element as HTMLInputElement).value).toBe('封面')
    wrapper.findAll('form.asset-upload input').forEach((field) => {
      expect(field.attributes('disabled')).toBeUndefined()
    })
    wrapper.unmount()
  })

  it.each(['', 'pending'])('完成确认重试保留修改后的替代文本，状态筛选为 %s', /** 确认失败后编辑双语替代文本，重试成功并切回默认筛选仍须保留最新输入。 */ async (status) => {
    let completeAttempts = 0
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/assets/uploads')) {
        return jsonResponse({
          asset: { ...asset('asset_retry', 'pending') },
          uploadUrl: 'https://upload.example.test/signed',
          headers: { 'Content-Type': 'image/webp' },
          expiresAt: '2026-10-09T09:00:00Z',
        })
      }
      if (url === 'https://upload.example.test/signed') return new Response(null, { status: 200 })
      if (url.endsWith('/api/v1/assets/asset_retry/complete')) {
        completeAttempts++
        return completeAttempts === 1
          ? jsonResponse({ code: 'metadata_pending' }, 503)
          : jsonResponse(asset('asset_retry'))
      }
      return jsonResponse({ items: completeAttempts >= 2 ? [asset('asset_retry')] : [] })
    })
    vi.stubGlobal('fetch', fetcher)
    const wrapper = mountWorkbench()
    if (status) {
      await wrapper.get('[data-testid="asset-status-filter"]').setValue(status)
      await flushPromises()
    }
    const input = wrapper.get('[data-testid="asset-file-input"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['asset'], 'cover.webp', { type: 'image/webp' })],
    })
    await input.trigger('change')
    await wrapper.get('[data-testid="asset-source-zh"]').setValue('官方授权')
    await wrapper.get('[data-testid="asset-alt-upload-zh"]').setValue('重试后封面')

    await wrapper.get('form.asset-upload').trigger('submit')
    await flushPromises()
    const retry = wrapper.findAll('button').find(button => button.text().includes('重试确认'))
    expect(retry).toBeDefined()

    await wrapper.get('[data-testid="asset-alt-upload-zh"]').setValue('修改后的封面')
    await wrapper.get('form input[aria-label="替代文本 · 英文"]').setValue('Updated cover')

    await retry!.trigger('click')
    await flushPromises()

    if (status) {
      expect(wrapper.find('[data-asset-id="asset_retry"]').exists()).toBe(false)
      await wrapper.get('[data-testid="asset-status-filter"]').setValue('')
      await flushPromises()
    }

    const card = wrapper.get('[data-asset-id="asset_retry"]')
    expect((card.get('[data-testid="asset-alt-zh"]').element as HTMLInputElement).value).toBe('修改后的封面')
    expect((card.get('input[aria-label="替代文本 · 英文: asset_retry"]').element as HTMLInputElement).value).toBe('Updated cover')
    expect(card.get('[data-testid="asset-insert"]').attributes('disabled')).toBeUndefined()
  })
})

/** 使用真实状态机挂载素材工作台。 */
function mountWorkbench() {
  return mount(AssetWorkbench, {
    props: {
      locale: 'zh-CN',
      editorText: JSON.stringify(fixtureData),
      apiBaseUrl: 'https://api.yujian.me',
      token: 'session-token',
    },
  })
}

/** 构造素材列表和完成确认响应。 */
function asset(id: string, status: AdminAsset['status'] = 'ready'): AdminAsset {
  return {
    id,
    src: `https://media.yujian.me/assets/${id}/source.webp`,
    status,
    metadata: {
      fileName: `${id}.webp`,
      contentType: 'image/webp',
      declaredSize: 5,
      checksum: 'sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
      width: 1200,
      height: 1200,
    },
    rights: { source: { 'zh-CN': '官方授权' } },
    createdAt: '2026-10-09T08:00:00Z',
  }
}

/** 构造 JSON API 响应。 */
function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

/** 创建由测试显式完成的 Promise。 */
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((complete) => {
    resolve = complete
  })
  return { promise, resolve }
}
