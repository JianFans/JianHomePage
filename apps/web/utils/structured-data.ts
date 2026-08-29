import type { YujianContentSnapshot } from '@yujian/schema'
import type { SupportedLocale } from './localized'
import { resolveLocalized } from './localized'

type StructuredDataNode = Record<string, unknown> & {
  '@type': string
}

export interface HomepageStructuredData {
  '@context': 'https://schema.org'
  '@graph': StructuredDataNode[]
}

function isoDuration(seconds: number) {
  return `PT${Math.max(0, Math.round(seconds))}S`
}

function absoluteUrl(baseUrl: string, value: string) {
  try {
    return new URL(value, `${baseUrl.replace(/\/$/, '')}/`).href
  } catch {
    return value
  }
}

export function buildStructuredData(
  snapshot: YujianContentSnapshot,
  locale: SupportedLocale,
): HomepageStructuredData {
  const canonicalUrl = snapshot.site.canonicalUrl.replace(/\/$/, '')
  const artistId = `${canonicalUrl}/#artist`
  const assets = new Map(snapshot.assets.map(asset => [asset.id, asset]))
  const graph: StructuredDataNode[] = [
    {
      '@type': 'MusicGroup',
      '@id': artistId,
      name: resolveLocalized(snapshot.site.artistName, locale),
      url: canonicalUrl,
      sameAs: snapshot.site.socialLinks.map(link => link.url),
    },
  ]

  for (const track of snapshot.tracks) {
    graph.push({
      '@type': 'MusicRecording',
      '@id': `${canonicalUrl}/#${track.id}`,
      name: resolveLocalized(track.title, locale),
      duration: isoDuration(track.durationSeconds),
      byArtist: { '@id': artistId },
      url: track.platformLinks[0]?.url || canonicalUrl,
    })
  }

  for (const video of snapshot.videos) {
    const poster = assets.get(video.posterAssetId)
    graph.push({
      '@type': 'VideoObject',
      '@id': `${canonicalUrl}/#${video.id}`,
      name: resolveLocalized(video.title, locale),
      duration: isoDuration(video.durationSeconds),
      thumbnailUrl: poster ? absoluteUrl(canonicalUrl, poster.src) : undefined,
      contentUrl: video.platformLinks[0]?.url,
      creator: { '@id': artistId },
    })
  }

  for (const event of snapshot.events) {
    graph.push({
      '@type': 'MusicEvent',
      '@id': `${canonicalUrl}/#${event.id}`,
      name: resolveLocalized(event.title, locale),
      startDate: event.dateTime,
      eventStatus: 'https://schema.org/EventScheduled',
      url: event.detailUrl,
      performer: { '@id': artistId },
      location: {
        '@type': 'Place',
        name: resolveLocalized(event.venue, locale),
        address: resolveLocalized(event.city, locale),
      },
    })
  }

  return {
    '@context': 'https://schema.org',
    '@graph': graph,
  }
}

export function serializeStructuredData(data: HomepageStructuredData) {
  return JSON.stringify(data)
    .replace(/&/g, '\\u0026')
    .replace(/</g, '\\u003C')
    .replace(/>/g, '\\u003E')
}
