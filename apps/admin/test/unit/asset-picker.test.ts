import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi, afterEach } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import AssetPicker from '../../components/AssetPicker.vue'
import type { YujianContentSnapshot } from '@yujian/schema'

afterEach(() => { vi.unstubAllGlobals() })

describe('统一素材选择器', () => {
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
