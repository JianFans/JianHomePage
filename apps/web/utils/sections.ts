import type {
  ArtistProfile,
  ArtistSection,
  Event,
  EventSection,
  HeroSection,
  HeroSlide,
  Moment,
  MomentSection,
  MusicSection,
  Release,
  Video,
  VideoSection,
  YujianContentSnapshot,
} from '@yujian/schema'

type WithItems<TSection, TItem> = TSection & { items: TItem[] }

export type ResolvedHomepageSection =
  | WithItems<HeroSection, HeroSlide>
  | WithItems<MusicSection, Release>
  | WithItems<VideoSection, Video>
  | WithItems<EventSection, Event>
  | WithItems<MomentSection, Moment>
  | WithItems<ArtistSection, ArtistProfile>

function orderedItems<T extends { id: string }>(ids: string[], items: T[]) {
  const byId = new Map(items.map(item => [item.id, item]))
  return ids.flatMap(id => byId.get(id) ?? [])
}

export function resolveHomepageSections(
  snapshot: YujianContentSnapshot,
  _locale: 'zh-CN' | 'en',
  now = new Date(),
): ResolvedHomepageSection[] {
  return snapshot.homepage.sections.flatMap((section): ResolvedHomepageSection[] => {
    if (!section.enabled) {
      return []
    }

    switch (section.type) {
      case 'hero': {
        const items = orderedItems(section.itemIds, snapshot.heroSlides)
        return items.length ? [{ ...section, items }] : []
      }
      case 'music': {
        const items = orderedItems(section.itemIds, snapshot.releases).slice(0, section.limit)
        return items.length ? [{ ...section, items }] : []
      }
      case 'video': {
        const items = orderedItems(section.itemIds, snapshot.videos).slice(0, section.limit)
        return items.length ? [{ ...section, items }] : []
      }
      case 'event': {
        const items = orderedItems(section.itemIds, snapshot.events)
          .filter(event => event.status === 'scheduled' && new Date(event.dateTime) > now)
          .slice(0, section.limit)
        return items.length ? [{ ...section, items }] : []
      }
      case 'moment': {
        const items = orderedItems(section.itemIds, snapshot.moments).slice(0, section.limit)
        return items.length ? [{ ...section, items }] : []
      }
      case 'artist': {
        const items = section.itemIds[0] === snapshot.artist.id ? [snapshot.artist] : []
        return items.length ? [{ ...section, items }] : []
      }
    }
  })
}
