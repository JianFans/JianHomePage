import Ajv2020, { type ErrorObject } from 'ajv/dist/2020.js'
import schema from '../schema/content-snapshot.schema.json'
import type {
  Asset,
  Event,
  HeroSlide,
  Moment,
  Release,
  Track,
  Video,
  YujianContentSnapshot,
} from './generated'

const ajv = new Ajv2020({ allErrors: true, strict: true, validateFormats: true })
ajv.addFormat('date', { type: 'string', validate: isISODate })
ajv.addFormat('date-time', { type: 'string', validate: isRFC3339DateTime })
ajv.addFormat('https-url', { type: 'string', validate: isHTTPSURL })
const validateSchema = ajv.compile(schema)

export class ContentSnapshotValidationError extends Error {
  readonly issues: readonly string[]

  constructor(issues: readonly string[]) {
    super(`Content snapshot validation failed at ${issues[0] ?? '/'}`)
    this.name = 'ContentSnapshotValidationError'
    this.issues = issues
  }
}

export function validateContentSnapshot(value: unknown): readonly string[] {
  if (!validateSchema(value)) {
    return (validateSchema.errors ?? []).map(formatSchemaIssue)
  }
  return validateSemantics(value as unknown as YujianContentSnapshot)
}

export function assertContentSnapshot(value: unknown): asserts value is YujianContentSnapshot {
  const issues = validateContentSnapshot(value)
  if (issues.length > 0) {
    throw new ContentSnapshotValidationError(issues)
  }
}

function formatSchemaIssue(error: ErrorObject): string {
  if (error.keyword === 'required') {
    const missing = String((error.params as { missingProperty?: string }).missingProperty ?? '')
    return `${error.instancePath}/${escapePointer(missing)}`
  }
  if (error.keyword === 'additionalProperties') {
    const property = String((error.params as { additionalProperty?: string }).additionalProperty ?? '')
    return `${error.instancePath}/${escapePointer(property)}`
  }
  return error.instancePath || '/'
}

function validateSemantics(snapshot: YujianContentSnapshot): string[] {
  const issues: string[] = []
  const assets = indexRecords(snapshot.assets, '/assets', issues)
  const heroes = indexRecords(snapshot.heroSlides, '/heroSlides', issues)
  const releases = indexRecords(snapshot.releases, '/releases', issues)
  const tracks = indexRecords(snapshot.tracks, '/tracks', issues)
  const videos = indexRecords(snapshot.videos, '/videos', issues)
  const events = indexRecords(snapshot.events, '/events', issues)
  const moments = indexRecords(snapshot.moments, '/moments', issues)
  const contentIds = renderedContentIds(snapshot, { heroes, releases, videos, events, moments })

  requireAssetKind(snapshot.site.seo.ogAssetId, assets, ['image', 'gif'], '/site/seo/ogAssetId', issues)
  validateHomepage(snapshot, { heroes, releases, videos, events, moments }, issues)
  validateHeroes(snapshot.heroSlides, assets, releases, contentIds, issues)
  validateReleases(snapshot.releases, assets, tracks, issues)
  validateTracks(snapshot.tracks, assets, releases, issues)
  validateVideos(snapshot.videos, assets, issues)
  validateEvents(snapshot.events, assets, issues)
  validateMoments(snapshot.moments, assets, contentIds, issues)
  requireAssetKind(snapshot.artist.portraitAssetId, assets, ['image', 'gif'], '/artist/portraitAssetId', issues)
  snapshot.assets.forEach((asset, index) => {
    if (asset.posterAssetId) {
      requireAssetKind(asset.posterAssetId, assets, ['image', 'gif'], `/assets/${index}/posterAssetId`, issues)
    }
  })
  return issues
}

function indexRecords<T extends { id: string }>(records: readonly T[], base: string, issues: string[]): Map<string, T> {
  const result = new Map<string, T>()
  records.forEach((record, index) => {
    if (result.has(record.id)) {
      issues.push(`${base}/${index}/id`)
    } else {
      result.set(record.id, record)
    }
  })
  return result
}

interface HomepageIndexes {
  heroes: ReadonlyMap<string, HeroSlide>
  releases: ReadonlyMap<string, Release>
  videos: ReadonlyMap<string, Video>
  events: ReadonlyMap<string, Event>
  moments: ReadonlyMap<string, Moment>
}

function renderedContentIds(snapshot: YujianContentSnapshot, indexes: HomepageIndexes): Set<string> {
  const result = new Set<string>()
  const referenceTime = Date.parse(snapshot.generatedAt)

  snapshot.homepage.sections.forEach((section) => {
    if (!section.enabled) return

    if (section.type === 'hero') {
      section.itemIds.forEach((id) => {
        const slide = indexes.heroes.get(id)
        const startsAt = slide?.startsAt ? Date.parse(slide.startsAt) : Number.NEGATIVE_INFINITY
        const endsAt = slide?.endsAt ? Date.parse(slide.endsAt) : Number.POSITIVE_INFINITY
        if (slide && startsAt <= referenceTime && referenceTime < endsAt) result.add(id)
      })
      return
    }

    if (section.type === 'music') {
      section.itemIds.slice(0, section.limit).forEach((id) => {
        const release = indexes.releases.get(id)
        if (!release) return
        result.add(id)
        release.trackIds.forEach(trackId => result.add(trackId))
      })
      return
    }

    if (section.type === 'video') {
      section.itemIds.slice(0, section.limit).forEach(id => result.add(id))
      return
    }

    if (section.type === 'event') {
      section.itemIds
        .flatMap(id => indexes.events.get(id) ?? [])
        .filter(event => event.status === 'scheduled' && Date.parse(event.dateTime) > referenceTime)
        .slice(0, section.limit)
        .forEach(event => result.add(event.id))
      return
    }

    if (section.type === 'moment') {
      section.itemIds.slice(0, section.limit).forEach(id => result.add(id))
      return
    }

    if (section.itemIds[0] === snapshot.artist.id) result.add(snapshot.artist.id)
  })

  return result
}

function validateHomepage(snapshot: YujianContentSnapshot, indexes: HomepageIndexes, issues: string[]) {
  snapshot.homepage.sections.forEach((section, sectionIndex) => {
    const base = `/homepage/sections/${sectionIndex}/itemIds`
    if (section.type === 'artist') {
      section.itemIds.forEach((id, itemIndex) => {
        if (id !== snapshot.artist.id) issues.push(`${base}/${itemIndex}`)
      })
      return
    }
    const index = section.type === 'hero'
      ? indexes.heroes
      : section.type === 'music'
        ? indexes.releases
        : section.type === 'video'
          ? indexes.videos
          : section.type === 'event'
            ? indexes.events
            : indexes.moments
    section.itemIds.forEach((id, itemIndex) => requireReference(id, index, `${base}/${itemIndex}`, issues))
  })
}

function validateHeroes(
  records: readonly HeroSlide[],
  assets: ReadonlyMap<string, Asset>,
  releases: ReadonlyMap<string, Release>,
  contentIds: ReadonlySet<string>,
  issues: string[],
) {
  records.forEach((record, index) => {
    const base = `/heroSlides/${index}`
    requireAssetKind(record.assetId, assets, [record.mediaKind], `${base}/assetId`, issues)
    if (record.mobileAssetId) requireAssetKind(record.mobileAssetId, assets, [record.mediaKind], `${base}/mobileAssetId`, issues)
    if (record.posterAssetId) requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `${base}/posterAssetId`, issues)
    if (record.releaseId) requireReference(record.releaseId, releases, `${base}/releaseId`, issues)
    validateInternalTarget(record.target, contentIds, `${base}/target/contentId`, issues)
  })
}

function validateReleases(
  records: readonly Release[],
  assets: ReadonlyMap<string, Asset>,
  tracks: ReadonlyMap<string, Track>,
  issues: string[],
) {
  records.forEach((record, index) => {
    const base = `/releases/${index}`
    requireAssetKind(record.coverAssetId, assets, ['image', 'gif'], `${base}/coverAssetId`, issues)
    record.trackIds.forEach((trackId, trackIndex) => {
      requireReference(trackId, tracks, `${base}/trackIds/${trackIndex}`, issues)
      const track = tracks.get(trackId)
      if (track && track.releaseId !== record.id) issues.push(`${base}/trackIds/${trackIndex}`)
    })
  })
}

function validateTracks(
  records: readonly Track[],
  assets: ReadonlyMap<string, Asset>,
  releases: ReadonlyMap<string, Release>,
  issues: string[],
) {
  records.forEach((record, index) => {
    const base = `/tracks/${index}`
    requireReference(record.releaseId, releases, `${base}/releaseId`, issues)
    if (record.previewAssetId) requireAssetKind(record.previewAssetId, assets, ['audio'], `${base}/previewAssetId`, issues)
  })
}

function validateVideos(records: readonly Video[], assets: ReadonlyMap<string, Asset>, issues: string[]) {
  records.forEach((record, index) => {
    const base = `/videos/${index}`
    requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `${base}/posterAssetId`, issues)
    if (record.videoAssetId) requireAssetKind(record.videoAssetId, assets, ['video'], `${base}/videoAssetId`, issues)
  })
}

function validateEvents(records: readonly Event[], assets: ReadonlyMap<string, Asset>, issues: string[]) {
  records.forEach((record, index) => {
    if (record.posterAssetId) requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `/events/${index}/posterAssetId`, issues)
  })
}

function validateMoments(
  records: readonly Moment[],
  assets: ReadonlyMap<string, Asset>,
  contentIds: ReadonlySet<string>,
  issues: string[],
) {
  records.forEach((record, index) => {
    requireAssetKind(record.assetId, assets, ['image', 'gif'], `/moments/${index}/assetId`, issues)
    validateInternalTarget(record.target, contentIds, `/moments/${index}/target/contentId`, issues)
  })
}

function validateInternalTarget(
  target: HeroSlide['target'] | Moment['target'],
  contentIds: ReadonlySet<string>,
  path: string,
  issues: string[],
) {
  if (target?.kind === 'internal' && !contentIds.has(target.contentId)) issues.push(path)
}

function requireReference(id: string, records: ReadonlyMap<string, unknown>, path: string, issues: string[]) {
  if (!records.has(id)) issues.push(path)
}

function requireAssetKind(
  id: string,
  assets: ReadonlyMap<string, Asset>,
  allowedKinds: readonly Asset['kind'][],
  path: string,
  issues: string[],
) {
  const asset = assets.get(id)
  if (!asset || !allowedKinds.includes(asset.kind)) issues.push(path)
}

function escapePointer(value: string): string {
  return value.replaceAll('~', '~0').replaceAll('/', '~1')
}

function isISODate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return false
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const parsed = new Date(Date.UTC(year, month - 1, day))
  return parsed.getUTCFullYear() === year && parsed.getUTCMonth() === month - 1 && parsed.getUTCDate() === day
}

function isRFC3339DateTime(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)
    && !Number.isNaN(Date.parse(value))
}

function isHTTPSURL(value: string): boolean {
  if (value.trim() !== value || value.includes('\\')) return false
  const authority = value.slice('https://'.length).split(/[/?#]/, 1)[0] ?? ''
  if (authority.includes('@')) return false
  try {
    const parsed = new URL(value)
    const port = parsed.port === '' ? 443 : Number(parsed.port)
    return parsed.protocol === 'https:'
      && parsed.hostname.length > 0
      && parsed.username === ''
      && parsed.password === ''
      && Number.isInteger(port)
      && port >= 1
      && port <= 65_535
  } catch {
    return false
  }
}
