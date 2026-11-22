import type { Event, YujianContentSnapshot } from '@yujian/schema'
import type { SupportedLocale } from './localized'
import { resolveLocalized } from './localized'

export const EVENT_CATALOG_PATH = '/events/'

export const eventCatalogLabels = {
  'zh-CN': {
    heading: '全部现场',
    empty: '暂无现场信息',
    filteredEmpty: '此分类下暂无现场信息',
    filter: '筛选现场',
    all: '全部',
    upcomingFilter: '即将举行',
    pastFilter: '往期与取消',
    upcoming: '即将举行',
    soldOut: '已售罄',
    cancelled: '已取消',
    ended: '已结束',
  },
  en: {
    heading: 'Live events',
    empty: 'No live events yet',
    filteredEmpty: 'No events in this category',
    filter: 'Filter events',
    all: 'All',
    upcomingFilter: 'Upcoming',
    pastFilter: 'Past & cancelled',
    upcoming: 'Upcoming',
    soldOut: 'Sold out',
    cancelled: 'Cancelled',
    ended: 'Past event',
  },
}

export function eventCatalogUrl(canonicalUrl: string): string {
  return new URL(EVENT_CATALOG_PATH, canonicalUrl).href
}

export function eventCatalogMetadata(snapshot: YujianContentSnapshot, locale: SupportedLocale) {
  const artist = resolveLocalized(snapshot.site.artistName, locale)
  const brand = resolveLocalized(snapshot.site.brand, locale)
  return {
    title: `${eventCatalogLabels[locale].heading} · ${artist} | ${brand}`,
    description: locale === 'en' ? `Live event dates and venues for ${artist}.` : `${artist}的演出时间、地点与现场信息。`,
    canonical: eventCatalogUrl(snapshot.site.canonicalUrl),
  }
}

/** Sort upcoming events first by date, then historical and cancelled events newest first. */
export function orderEvents(snapshot: YujianContentSnapshot): Event[] {
  const referenceTime = Date.parse(snapshot.generatedAt)
  return [...snapshot.events].sort((left, right) => {
    const leftUpcoming = left.status !== 'cancelled' && Date.parse(left.dateTime) > referenceTime
    const rightUpcoming = right.status !== 'cancelled' && Date.parse(right.dateTime) > referenceTime
    if (leftUpcoming !== rightUpcoming) return leftUpcoming ? -1 : 1

    const dateOrder = Date.parse(left.dateTime) - Date.parse(right.dateTime)
    return leftUpcoming ? dateOrder : -dateOrder
  })
}
