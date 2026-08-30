import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'
import { afterEach, expect, it, vi } from 'vitest'

const originalSnapshotPath = process.env.CONTENT_SNAPSHOT_PATH

afterEach(() => {
  if (originalSnapshotPath === undefined) {
    delete process.env.CONTENT_SNAPSHOT_PATH
  }
  else {
    process.env.CONTENT_SNAPSHOT_PATH = originalSnapshotPath
  }
  vi.resetModules()
})

it('从发布快照生成静态 SEO head', async () => {
  const fixturePath = resolve(process.cwd(), '../../content/fixtures/homepage.json')
  const snapshot = JSON.parse(readFileSync(fixturePath, 'utf8'))
  snapshot.site.canonicalUrl = 'https://release.yujian.me'
  snapshot.site.seo.title['zh-CN'] = '发布快照标题'
  snapshot.site.seo.description['zh-CN'] = '发布快照描述'
  snapshot.site.seo.ogAssetId = 'asset_hero_stage'
  const root = mkdtempSync(join(tmpdir(), 'yujian-nuxt-config-'))
  const snapshotPath = join(root, 'release.json')
  writeFileSync(snapshotPath, JSON.stringify(snapshot))
  process.env.CONTENT_SNAPSHOT_PATH = snapshotPath
  vi.resetModules()

  const { loadNuxtConfig } = await import('nuxt/kit')
  const config = await loadNuxtConfig({ cwd: process.cwd() })
  const head = config.app?.head as {
    title?: string
    meta?: Array<Record<string, string>>
    link?: Array<Record<string, string>>
  }
  const metaByName = new Map(head.meta?.flatMap(meta => {
    const key = meta.name ?? meta.property
    return key ? [[key, meta.content] as const] : []
  }))

  expect(head.title).toBe('发布快照标题')
  expect(metaByName.get('description')).toBe('发布快照描述')
  expect(metaByName.get('og:title')).toBe('发布快照标题')
  expect(metaByName.get('og:description')).toBe('发布快照描述')
  expect(metaByName.get('og:url')).toBe('https://release.yujian.me')
  expect(metaByName.get('og:image')).toBe('https://release.yujian.me/media/hero-stage.webp')
  expect(head.link?.find(link => link.rel === 'canonical')?.href).toBe('https://release.yujian.me')
  expect(config.nitro?.prerender?.routes).toEqual(expect.arrayContaining(['/robots.txt', '/sitemap.xml']))
})
