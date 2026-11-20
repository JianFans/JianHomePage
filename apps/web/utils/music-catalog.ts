import type { YujianContentSnapshot } from '@yujian/schema'
import type { SupportedLocale } from './localized'
import { resolveLocalized } from './localized'

export const MUSIC_CATALOG_PATH = '/music/'

export const musicCatalogLabels = {
  'zh-CN': { heading: '全部音乐作品', empty: '暂无音乐作品', preview: '试听' },
  en: { heading: 'All music', empty: 'No music releases yet', preview: 'Preview ' },
}

export function musicCatalogUrl(canonicalUrl: string): string {
  return new URL(MUSIC_CATALOG_PATH, canonicalUrl).href
}

export function musicCatalogMetadata(snapshot: YujianContentSnapshot, locale: SupportedLocale) {
  const artist = resolveLocalized(snapshot.site.artistName, locale)
  const brand = resolveLocalized(snapshot.site.brand, locale)
  return {
    title: `${musicCatalogLabels[locale].heading} · ${artist} | ${brand}`,
    description: locale === 'en' ? `Music releases and tracks by ${artist}.` : `${artist}的音乐作品与曲目。`,
    canonical: musicCatalogUrl(snapshot.site.canonicalUrl),
  }
}
