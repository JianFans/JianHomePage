import { createSSRApp } from 'vue'
import { renderToString } from '@vue/server-renderer'
import type { YujianContentSnapshot } from '@yujian/schema'
import App from './App.vue'
import { buildStructuredData, serializeStructuredData } from '../utils/structured-data'
import { renderRobots, renderSitemap } from '../utils/crawler-files'

declare const __SITE_ASSETS__: { script: string, styles: string[] }

/** Escape attribute and text contexts; JSON script payloads use a separate serializer. */
function escapeHTML(value: string) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;')
}

/** A closed, bundled entry point: snapshots are data and no modules or processes are loaded. */
async function render(raw: string) {
  const snapshot: YujianContentSnapshot = JSON.parse(raw)
  const locale = snapshot.site.defaultLocale
  const body = await renderToString(createSSRApp(App, { snapshot }))
  const seo = snapshot.site.seo
  const asset = snapshot.assets.find(item => item.id === seo.ogAssetId)!
  const title = escapeHTML(seo.title[locale])
  const description = escapeHTML(seo.description[locale])
  const canonical = escapeHTML(snapshot.site.canonicalUrl)
  const image = escapeHTML(new URL(asset.src, snapshot.site.canonicalUrl).href)
  const data = JSON.stringify(snapshot).replaceAll('<', '\\u003c').replaceAll('>', '\\u003e').replaceAll('&', '\\u0026')
  return {
    html: [
      `<!doctype html><html lang="${locale}"><head>`,
      '<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">',
      '<link rel="icon" type="image/svg+xml" href="/site/favicon.svg">',
      `<title>${title}</title><meta name="description" content="${description}">`,
      `<link rel="canonical" href="${canonical}"><meta property="og:title" content="${title}">`,
      `<meta property="og:description" content="${description}"><meta property="og:type" content="website">`,
      `<meta property="og:url" content="${canonical}"><meta property="og:image" content="${image}">`,
      `<meta property="og:image:alt" content="${escapeHTML(asset.alt[locale])}"><meta name="twitter:card" content="summary_large_image">`,
      ...__SITE_ASSETS__.styles.map(path => `<link rel="stylesheet" href="${path}">`),
      `<script type="application/ld+json">${serializeStructuredData(buildStructuredData(snapshot, locale))}</script>`,
      `</head><body><div id="app">${body}</div>`,
      `<script type="application/json" id="yujian-snapshot">${data}</script>`,
      `<script type="module" src="${__SITE_ASSETS__.script}"></script></body></html>`,
    ].join(''),
    robots: renderRobots(snapshot.site.canonicalUrl),
    sitemap: renderSitemap(snapshot.site.canonicalUrl),
  }
}

Object.assign(globalThis, { __render: render })
