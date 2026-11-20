import type { YujianContentSnapshot } from '@yujian/schema'
import type { SupportedLocale } from './localized'
import { resolveLocalized } from './localized'

export const VIDEO_CATALOG_PATH = '/video/'

export const videoCatalogLabels = {
  'zh-CN': { heading: '全部影像', empty: '暂无影像作品', open: '打开影像' },
  en: { heading: 'All videos', empty: 'No videos yet', open: 'Open video' },
}

export function videoCatalogUrl(canonicalUrl: string): string {
  return new URL(VIDEO_CATALOG_PATH, canonicalUrl).href
}

export function videoCatalogMetadata(snapshot: YujianContentSnapshot, locale: SupportedLocale) {
  const artist = resolveLocalized(snapshot.site.artistName, locale)
  const brand = resolveLocalized(snapshot.site.brand, locale)
  return {
    title: `${videoCatalogLabels[locale].heading} · ${artist} | ${brand}`,
    description: locale === 'en' ? `Videos and live performances by ${artist}.` : `${artist}的影像作品与现场视频。`,
    canonical: videoCatalogUrl(snapshot.site.canonicalUrl),
  }
}
