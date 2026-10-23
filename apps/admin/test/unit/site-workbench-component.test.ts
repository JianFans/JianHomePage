import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json'
import SiteWorkbench from '../../components/SiteWorkbench.vue'
import { createSiteDraft } from '../../utils/site-workbench'

const snapshot = fixture as unknown as YujianContentSnapshot

describe('站点资料表单', () => {
  it('编辑网址与可选图片，清空图片后移除对应键', async () => {
    const draft = createSiteDraft(snapshot)
    const wrapper = mount(SiteWorkbench, { props: { draft, snapshot, locale: 'zh-CN', locked: false, stale: false, issues: [] } })
    try {
      await wrapper.get('[data-testid="site-canonical"]').setValue('https://new.yujian.me')
      expect(draft.site.canonicalUrl).toBe('https://new.yujian.me')
      const image = snapshot.assets.find(asset => asset.kind === 'image')!
      await wrapper.get('[data-testid="site-portrait"]').setValue(image.id)
      expect(draft.artist.portraitAssetId).toBe(image.id)
      await wrapper.get('[data-testid="site-portrait"]').setValue('')
      await wrapper.get('[data-testid="site-og-image"]').setValue('')
      expect(Object.hasOwn(draft.artist, 'portraitAssetId')).toBe(false)
      expect(Object.hasOwn(draft.site.seo, 'ogAssetId')).toBe(false)
      expect(snapshot.site.canonicalUrl).not.toBe('https://new.yujian.me')
    } finally { wrapper.unmount() }
  })

  it('把完整快照中的资料诊断关联到实际可读的字段说明', () => {
    const paths = ['/site/canonicalUrl', '/site/seo/ogAssetId', '/artist/portraitAssetId']
    const wrapper = mount(SiteWorkbench, { props: {
      draft: createSiteDraft(snapshot), snapshot, locale: 'en', locked: false, stale: false,
      issues: paths.map(path => ({ path, source: 'semantic' as const, code: 'missing-reference' })),
    } })
    try {
      for (const path of paths) {
        const control = wrapper.get(`[data-snapshot-path="${path}"]`)
        expect(control.attributes('aria-invalid')).toBe('true')
        const id = control.attributes('aria-describedby')
        const description = wrapper.findAll('[id]').find(node => node.attributes('id') === id)
        expect(description?.text()).toContain(path)
        expect(description?.text()).toContain('Reference not found')
      }
    } finally { wrapper.unmount() }
  })
})
