import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, afterEach } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import AssetPicker from '../../components/AssetPicker.vue'
import type { YujianContentSnapshot } from '@yujian/schema'

afterEach(() => { vi.unstubAllGlobals() })

describe('统一素材选择器', () => {
  it.each([
    { locked: false, kind: 'image' as const, accepted: true },
    { locked: true, kind: 'image' as const, accepted: false },
    { locked: false, kind: 'audio' as const, accepted: false },
  ])('重复选择复用快照记录并保留锁定和类型校验：%j', async ({ locked, kind, accepted }) => {
    const snapshot = structuredClone(fixture) as unknown as YujianContentSnapshot
    const existing = snapshot.assets.find(asset => asset.kind === kind)!
    const libraryAsset = {
      id: existing.id, status: 'ready', src: existing.src,
      metadata: { fileName: 'cover.webp', contentType: 'image/webp', declaredSize: existing.byteSize, checksum: existing.checksum, width: existing.width, height: existing.height },
      rights: existing.rights,
    }
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ items: [libraryAsset] }))))
    const wrapper = mount(AssetPicker, { props: { open: true, snapshot, kinds: ['image'], locale: 'en', label: 'Cover', apiBaseUrl: '', csrfToken: 'test', locked } })
    try {
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      await flushPromises()
      await wrapper.get('[data-testid="asset-alt-zh"]').setValue('素材库中新填的替代文本')
      await wrapper.get('[data-testid="asset-insert"]').trigger('click')
      if (accepted) expect(wrapper.emitted('select')?.[0]?.[0]).toEqual(existing)
      else expect(wrapper.emitted('select')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('切换素材来源保留文件和上传输入，关闭前可取消丢弃', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ items: [] }))))
    const wrapper = mount(AssetPicker, { props: { open: true, snapshot: fixture as unknown as YujianContentSnapshot, kinds: ['image'], locale: 'en', label: 'Cover', apiBaseUrl: '', csrfToken: 'test', locked: false } })
    try {
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      const input = wrapper.get('[data-testid="asset-file-input"]')
      Object.defineProperty(input.element, 'files', { value: [new File(['asset'], 'cover.webp', { type: 'image/webp' })], configurable: true })
      await input.trigger('change')
      await wrapper.get('[data-testid="asset-source-zh"]').setValue('授权来源')
      await wrapper.get('[data-testid="asset-alt-upload-zh"]').setValue('上传封面')
      await wrapper.get('button[aria-pressed="false"]').trigger('click')
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      expect(wrapper.get('[data-testid="asset-source-zh"]').element).toHaveProperty('value', '授权来源')
      expect(wrapper.get('[data-testid="asset-alt-upload-zh"]').element).toHaveProperty('value', '上传封面')
      expect(wrapper.text()).toContain('cover.webp')
      await wrapper.get('[data-testid="dialog-close"]').trigger('click')
      expect(wrapper.emitted('close')).toBeUndefined()
      await wrapper.get('[data-testid="picker-keep"]').trigger('click')
      expect(wrapper.get('[data-testid="asset-source-zh"]').isVisible()).toBe(true)
      await wrapper.get('[data-testid="dialog-close"]').trigger('click')
      await wrapper.get('[data-testid="picker-discard"]').trigger('click')
      expect(wrapper.emitted('close')).toHaveLength(1)
      expect(wrapper.emitted('select')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('切换来源后保留失败的完成确认，只重试确认而不再次直传', async () => {
    let uploads = 0
    let blobs = 0
    let confirmations = 0
    const asset = { id: 'asset_132b59fd-f9b4-47b4-acd1-bbfae60d66b1', status: 'ready', src: 'https://media.example/new.webp', metadata: { fileName: 'new.webp', contentType: 'image/webp', declaredSize: 5, checksum: 'sha256:' + 'a'.repeat(64), width: 64, height: 64 }, rights: { source: { 'zh-CN': '授权' } } }
    vi.stubGlobal('fetch', vi.fn(async (input) => {
      const url = String(input)
      if (url.endsWith('/assets/uploads')) { uploads++; return new Response(JSON.stringify({ asset: { ...asset, status: 'pending' }, uploadUrl: 'https://upload.example/signed', expiresAt: '2026-10-24T00:00:00Z' })) }
      if (url === 'https://upload.example/signed') { blobs++; return new Response(null, { status: 200 }) }
      if (url.endsWith('/complete')) { confirmations++; return new Response(JSON.stringify(confirmations === 1 ? { code: 'unavailable' } : asset), { status: confirmations === 1 ? 503 : 200 }) }
      return new Response(JSON.stringify({ items: [] }))
    }))
    const wrapper = mount(AssetPicker, { props: { open: true, snapshot: fixture as unknown as YujianContentSnapshot, kinds: ['image'], locale: 'en', label: 'Cover', apiBaseUrl: '', csrfToken: 'test', locked: false } })
    try {
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      const input = wrapper.get('[data-testid="asset-file-input"]')
      Object.defineProperty(input.element, 'files', { value: [new File(['asset'], 'cover.webp', { type: 'image/webp' })], configurable: true })
      await input.trigger('change')
      await wrapper.get('[data-testid="asset-source-zh"]').setValue('授权来源')
      await wrapper.get('[data-testid="asset-alt-upload-zh"]').setValue('上传封面')
      await wrapper.get('form.asset-upload').trigger('submit')
      await flushPromises()
      expect(wrapper.get('[data-testid="asset-stage"]').text()).toContain('Upload failed')
      await wrapper.get('button[aria-pressed="false"]').trigger('click')
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      const retry = wrapper.findAll('button').find(button => button.text() === 'Retry confirmation')
      expect(retry).toBeDefined()
      await retry!.trigger('click')
      await flushPromises()
      expect(wrapper.get('[data-testid="asset-stage"]').text()).toContain('Upload complete')
      expect(wrapper.get('[data-testid="asset-alt-zh"]').element).toHaveProperty('value', '上传封面')
      expect({ uploads, blobs, confirmations }).toEqual({ uploads: 1, blobs: 1, confirmations: 2 })
    } finally { wrapper.unmount() }
  })

  it('仅展示当前用途的素材，搜索与选择保留完整 canonical 记录', async () => {
    const wrapper = mount(AssetPicker, { props: { open: true, snapshot: fixture as unknown as YujianContentSnapshot, kinds: ['audio'], locale: 'en', label: 'Preview', apiBaseUrl: '', csrfToken: '', locked: false } })
    try {
      const rows = wrapper.findAll('[data-testid="picker-snapshot-asset"]')
      expect(rows).toHaveLength(fixture.assets.filter(asset => asset.kind === 'audio').length)
      await rows[0]!.trigger('click')
      expect(wrapper.emitted('select')?.[0]?.[0]).toEqual(fixture.assets.find(asset => asset.kind === 'audio'))
      await wrapper.get('[data-testid="picker-search"]').setValue('not-existing')
      expect(wrapper.findAll('[data-testid="picker-snapshot-asset"]')).toHaveLength(0)
    } finally { wrapper.unmount() }
  })

  it('服务端素材可直接选择，错误类型不能绑定，关闭时不发出选择', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ items: [
      { id: 'asset_132b59fd-f9b4-47b4-acd1-bbfae60d66b1', status: 'ready', src: 'https://media.example/new.webp', metadata: { fileName: 'new.webp', contentType: 'image/webp', declaredSize: 7, checksum: 'sha256:' + 'a'.repeat(64), width: 64, height: 64 }, rights: { source: { 'zh-CN': '授权' } } },
      { id: 'asset_ba753d17-7778-45f5-a8c4-d4c56a6e7368', status: 'ready', src: 'https://media.example/no.mp3', metadata: { fileName: 'no.mp3', contentType: 'audio/mpeg', declaredSize: 7, checksum: 'sha256:' + 'b'.repeat(64) }, rights: { source: { 'zh-CN': '授权' } } },
    ] }))))
    const wrapper = mount(AssetPicker, { props: { open: true, snapshot: fixture as unknown as YujianContentSnapshot, kinds: ['image'], locale: 'en', label: 'Cover', apiBaseUrl: '', csrfToken: 'test', locked: false } })
    try {
      await wrapper.get('[data-testid="picker-library"]').trigger('click')
      await flushPromises()
      expect(wrapper.findAll('[data-testid="asset-card"]')).toHaveLength(1)
      await wrapper.get('[data-testid="asset-alt-zh"]').setValue('新封面')
      await wrapper.get('[data-testid="asset-insert"]').trigger('click')
      expect(wrapper.emitted('select')?.[0]?.[0]).toMatchObject({ kind: 'image', alt: { 'zh-CN': '新封面' } })
      expect(wrapper.emitted('update:editorText')).toBeUndefined()
    } finally { wrapper.unmount() }
  })
})
