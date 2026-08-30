import { expect, it } from 'vitest'
import { renderRobots, renderSitemap } from '../../utils/crawler-files'

const canonicalUrl = 'https://release.yujian.me'

it('从发布快照 canonical 生成 sitemap', () => {
  const body = renderSitemap(canonicalUrl)
  expect(body).toContain(`<loc>${canonicalUrl}/</loc>`)
})

it('从发布快照 canonical 生成 robots', () => {
  const body = renderRobots(canonicalUrl)
  expect(body).toContain(`Sitemap: ${canonicalUrl}/sitemap.xml`)
})
