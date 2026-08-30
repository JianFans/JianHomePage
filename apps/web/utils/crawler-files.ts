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
  return `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>${homepageUrl}</loc>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
</urlset>`
}

export function renderRobots(canonicalUrl: string): string {
  return `User-agent: *
Allow: /

Sitemap: ${canonicalBase(canonicalUrl)}/sitemap.xml`
}
