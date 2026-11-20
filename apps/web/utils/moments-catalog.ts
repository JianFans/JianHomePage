import type { YujianContentSnapshot } from '@yujian/schema'
import type { SupportedLocale } from './localized'
import { resolveLocalized } from './localized'
import { resolveHomepageSections } from './sections'

export const MOMENT_CATALOG_PATH = '/moments/'

export const momentCatalogLabels = {
  'zh-CN': {
    heading: '全部片段',
    empty: '暂无片段',
    open: '打开片段关联内容',
  },
  en: {
    heading: 'Moments',
    empty: 'No moments yet',
    open: 'Open linked moment content',
  },
}

export function momentCatalogUrl(canonicalUrl: string): string {
  return new URL(MOMENT_CATALOG_PATH, canonicalUrl).href
}

export function momentCatalogMetadata(snapshot: YujianContentSnapshot, locale: SupportedLocale) {
  const artist = resolveLocalized(snapshot.site.artistName, locale)
  const brand = resolveLocalized(snapshot.site.brand, locale)
  return {
    title: `${momentCatalogLabels[locale].heading} · ${artist} | ${brand}`,
    description: locale === 'en' ? `Photos and moments from ${artist}.` : `${artist}的现场片段与记录。`,
    canonical: momentCatalogUrl(snapshot.site.canonicalUrl),
  }
}

export function internalContentUrl(snapshot: YujianContentSnapshot, contentId: string): string | null {
  const id = encodeURIComponent(contentId)
  const homepageSections = resolveHomepageSections(snapshot, snapshot.site.defaultLocale)
  if (homepageSections.some(section => (section.type === 'artist' || section.type === 'hero') && section.items.some(item => item.id === contentId))) {
    return `/#${id}`
  }
  if (snapshot.releases.some(release => release.id === contentId) || snapshot.tracks.some(track => track.id === contentId)) {
    return `/music/#${id}`
  }
  if (snapshot.videos.some(video => video.id === contentId)) return `/video/#${id}`
  if (snapshot.events.some(event => event.id === contentId)) return `/events/#${id}`
  if (snapshot.moments.some(moment => moment.id === contentId)) return `${MOMENT_CATALOG_PATH}#${id}`
  return null
}
