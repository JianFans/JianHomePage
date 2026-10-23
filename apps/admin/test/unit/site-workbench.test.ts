import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { applySiteDraft, createSiteDraft } from '../../utils/site-workbench'
import type { YujianContentSnapshot } from '@yujian/schema'

const snapshot = fixture as unknown as YujianContentSnapshot
const text = JSON.stringify(snapshot)

describe('站点资料事务', () => {
  it('独立编辑资料，应用后保留作品、导航和语言约束', () => {
    const draft = createSiteDraft(snapshot)
    draft.site.brand['zh-CN'] = '新的遇健我'
    draft.artist.shortBio['zh-CN'] = '新的简介'
    expect(snapshot.site.brand['zh-CN']).not.toBe('新的遇健我')
    const result = applySiteDraft(text, text, draft)
    expect(result.error).toBeNull()
    const next = JSON.parse(result.text)
    expect(next.site.brand['zh-CN']).toBe('新的遇健我')
    expect(next.artist.shortBio['zh-CN']).toBe('新的简介')
    expect(next.releases).toEqual(snapshot.releases)
    expect(next.site.futureNavigation).toEqual(snapshot.site.futureNavigation)
  })

  it('拒绝过期基线，非法网址与错误图片类型均不部分应用', () => {
    const draft = createSiteDraft(snapshot)
    expect(applySiteDraft(text + ' ', text, draft)).toMatchObject({ text: text + ' ', error: 'stale-snapshot' })
    draft.site.canonicalUrl = 'javascript:alert(1)'
    const invalid = applySiteDraft(text, text, draft)
    expect(invalid.text).toBe(text)
    expect(invalid.error).toBe('invalid-draft')
    expect(invalid.issues.some(issue => issue.path.includes('canonicalUrl'))).toBe(true)
    draft.site.canonicalUrl = snapshot.site.canonicalUrl
    draft.artist.portraitAssetId = snapshot.assets.find(asset => asset.kind === 'audio')!.id
    expect(applySiteDraft(text, text, draft).error).toBe('invalid-draft')
  })

  it('不允许修改艺人 ID 或尚未开放的导航和语言配置', () => {
    const draft = createSiteDraft(snapshot)
    draft.artist.id = 'artist_15448d45-4d78-47af-9b3d-02d971893238'
    expect(applySiteDraft(text, text, draft).error).toBe('immutable-settings')
    draft.artist.id = snapshot.artist.id
    draft.site.futureNavigation = [{ id: 'nav_15448d45-4d78-47af-9b3d-02d971893238', label: { 'zh-CN': '论坛' }, url: 'https://example.com', enabled: true }]
    expect(applySiteDraft(text, text, draft).error).toBe('immutable-settings')
  })
})
