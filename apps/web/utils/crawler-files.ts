import { musicCatalogUrl } from './music-catalog'
import { videoCatalogUrl } from './video-catalog'
import { eventCatalogUrl } from './event-catalog'

function canonicalBase(canonicalUrl: string): string {
  return canonicalUrl.replace(/\/+$/, '')
}

function escapeXML(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&apos;')
}

export function renderSitemap(canonicalUrl: string): string {
  const homepageUrl = escapeXML(`${canonicalBase(canonicalUrl)}/`)
  const musicUrl = escapeXML(musicCatalogUrl(canonicalUrl))
  const videoUrl = escapeXML(videoCatalogUrl(canonicalUrl))
  const eventUrl = escapeXML(eventCatalogUrl(canonicalUrl))
  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>${homepageUrl}</loc>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
  <url>
    <loc>${musicUrl}</loc>
  </url>
  <url>
    <loc>${videoUrl}</loc>
  </url>
  <url>
    <loc>${eventUrl}</loc>
  </url>
</urlset>`
}

export function renderRobots(canonicalUrl: string): string {
  return `User-agent: *
Allow: /

Sitemap: ${canonicalBase(canonicalUrl)}/sitemap.xml`
}
