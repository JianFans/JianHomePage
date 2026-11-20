import { createSSRApp } from 'vue'
import { renderToString } from '@vue/server-renderer'
import type { YujianContentSnapshot } from '@yujian/schema'
import App from './App.vue'
import { buildStructuredData, serializeStructuredData } from '../utils/structured-data'
import { renderRobots, renderSitemap } from '../utils/crawler-files'
import { musicCatalogMetadata } from '../utils/music-catalog'
import { videoCatalogMetadata } from '../utils/video-catalog'
import { eventCatalogMetadata } from '../utils/event-catalog'
import { momentCatalogMetadata } from '../utils/moments-catalog'

declare const __SITE_ASSETS__: { script: string, styles: string[] }

/** Escape attribute and text contexts; JSON script payloads use a separate serializer. */
function escapeHTML(value: string) {
  return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&#39;')
}

/** A closed, bundled entry point: snapshots are data and no modules or processes are loaded. */
async function render(raw: string, snapshotURL: string) {
  const snapshot: YujianContentSnapshot = JSON.parse(raw)
  const locale = snapshot.site.defaultLocale
  const seo = snapshot.site.seo
  const asset = snapshot.assets.find(item => item.id === seo.ogAssetId)
  const imageMeta = asset
    ? `<meta property="og:image" content="${escapeHTML(new URL(asset.src, snapshot.site.canonicalUrl).href)}"><meta property="og:image:alt" content="${escapeHTML(asset.alt[locale])}">`
    : ''
  const homeMetadata = {
    title: seo.title[locale],
    description: seo.description[locale],
    canonical: snapshot.site.canonicalUrl,
  }

  async function renderPage(page: 'home' | 'music' | 'video' | 'events' | 'moments', metadata: typeof homeMetadata, snapshotURL: string) {
    const body = await renderToString(createSSRApp(App, { snapshot, page }))
    const title = escapeHTML(metadata.title)
    const description = escapeHTML(metadata.description)
    const canonical = escapeHTML(metadata.canonical)
    const structuredData = page === 'home'
      ? `<script type="application/ld+json">${serializeStructuredData(buildStructuredData(snapshot, locale))}</script>`
      : ''
    return [
      `<!doctype html><html lang="${locale}"><head>`,
      '<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">',
      '<link rel="icon" type="image/svg+xml" href="/site/favicon.svg">',
      `<title>${title}</title><meta name="description" content="${description}">`,
      `<link rel="canonical" href="${canonical}"><meta property="og:title" content="${title}">`,
      `<meta property="og:description" content="${description}"><meta property="og:type" content="website">`,
      `<meta property="og:url" content="${canonical}">${imageMeta}`,
      `<meta name="twitter:card" content="${asset ? 'summary_large_image' : 'summary'}">`,
      ...__SITE_ASSETS__.styles.map(path => `<link rel="stylesheet" href="${path}">`),
      structuredData,
      `</head><body><div id="app">${body}</div>`,
      `<script type="application/json" id="yujian-snapshot" data-page="${page}" data-src="${snapshotURL}"></script>`,
      `<script type="module" src="${__SITE_ASSETS__.script}"></script></body></html>`,
    ].join('')
  }

  return {
    html: await renderPage('home', homeMetadata, snapshotURL),
    music: await renderPage('music', musicCatalogMetadata(snapshot, locale), snapshotURL),
    video: await renderPage('video', videoCatalogMetadata(snapshot, locale), snapshotURL),
    events: await renderPage('events', eventCatalogMetadata(snapshot, locale), snapshotURL),
    moments: await renderPage('moments', momentCatalogMetadata(snapshot, locale), snapshotURL),
    robots: renderRobots(snapshot.site.canonicalUrl),
    sitemap: renderSitemap(snapshot.site.canonicalUrl),
  }
}

Object.assign(globalThis, { __render: render })
