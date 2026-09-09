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

const validationSchema = structuredClone(schema)
addSchemaDiscriminator(validationSchema, ['$defs', 'Homepage', 'properties', 'sections', 'items'], 'type')
addSchemaDiscriminator(validationSchema, ['$defs', 'ContentTarget'], 'kind')
addSchemaDiscriminator(validationSchema, ['$defs', 'Asset'], 'kind')

const ajv = new Ajv2020({ allErrors: true, strict: true, validateFormats: true, discriminator: true })
ajv.addFormat('date', { type: 'string', validate: isISODate })
ajv.addFormat('date-time', { type: 'string', validate: isRFC3339DateTime })
ajv.addFormat('https-url', { type: 'string', validate: isHTTPSURL })
const validateSchema = ajv.compile(validationSchema)

export type ContentSnapshotIssueSource = 'schema' | 'semantic'

export interface ContentSnapshotIssue {
  path: string
  source: ContentSnapshotIssueSource
  code: string
}

export class ContentSnapshotValidationError extends Error {
  readonly issues: readonly string[]

  /** 使用按校验顺序生成的问题路径构造可供上层识别的快照校验异常。 */
  constructor(issues: readonly string[]) {
    super(`Content snapshot validation failed at ${issues[0] ?? '/'}`)
    this.name = 'ContentSnapshotValidationError'
    this.issues = issues
  }
}

/**
 * 校验未知输入并返回稳定的 JSON Pointer 问题路径列表。
 */
export function validateContentSnapshot(value: unknown): readonly string[] {
  return diagnoseContentSnapshot(value).map(issue => issue.path)
}

/**
 * 依次执行 Schema 与语义校验，并返回带来源和代码的结构化诊断。
 */
export function diagnoseContentSnapshot(value: unknown): readonly ContentSnapshotIssue[] {
  if (!validateSchema(value)) {
    return (validateSchema.errors ?? []).map(error => ({
      path: formatSchemaIssue(error),
      source: 'schema',
      code: error.keyword,
    }))
  }
  return diagnoseSemantics(value as unknown as YujianContentSnapshot)
}

/**
 * 断言未知输入满足内容快照契约，否则抛出包含问题路径的异常。
 */
export function assertContentSnapshot(value: unknown): asserts value is YujianContentSnapshot {
  const issues = validateContentSnapshot(value)
  if (issues.length > 0) {
    throw new ContentSnapshotValidationError(issues)
  }
}

/** 将 Ajv 错误定位为稳定且已转义的 JSON Pointer。 */
function formatSchemaIssue(error: ErrorObject): string {
  if (error.keyword === 'discriminator') {
    const tag = String((error.params as { tag?: string }).tag ?? '')
    return tag ? `${error.instancePath}/${escapePointer(tag)}` : error.instancePath || '/'
  }
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

/**
 * 校验 Schema 无法表达的跨记录引用、素材类型和首页可见性约束。
 */
function diagnoseSemantics(snapshot: YujianContentSnapshot): ContentSnapshotIssue[] {
  const issues: ContentSnapshotIssue[] = []
  const assets = indexRecords(snapshot.assets, '/assets', issues)
  const heroes = indexRecords(snapshot.heroSlides, '/heroSlides', issues)
  const releases = indexRecords(snapshot.releases, '/releases', issues)
  const tracks = indexRecords(snapshot.tracks, '/tracks', issues)
  const videos = indexRecords(snapshot.videos, '/videos', issues)
  const events = indexRecords(snapshot.events, '/events', issues)
  const moments = indexRecords(snapshot.moments, '/moments', issues)
  const indexes = { heroes, releases, videos, events, moments }
  const knownContentIds = collectKnownContentIds(snapshot, indexes, tracks)
  const renderedIds = renderedContentIds(snapshot, indexes)

  requireAssetKind(snapshot.site.seo.ogAssetId, assets, ['image', 'gif'], '/site/seo/ogAssetId', issues)
  validateHomepage(snapshot, indexes, issues)
  validateHeroes(snapshot.heroSlides, assets, releases, knownContentIds, renderedIds, issues)
  validateReleases(snapshot.releases, assets, tracks, issues)
  validateTracks(snapshot.tracks, assets, releases, issues)
  validateVideos(snapshot.videos, assets, issues)
  validateEvents(snapshot.events, assets, issues)
  validateMoments(snapshot.moments, assets, knownContentIds, renderedIds, issues)
  requireAssetKind(snapshot.artist.portraitAssetId, assets, ['image', 'gif'], '/artist/portraitAssetId', issues)
  snapshot.assets.forEach((asset, index) => {
    if (asset.posterAssetId) {
      requireAssetKind(asset.posterAssetId, assets, ['image', 'gif'], `/assets/${index}/posterAssetId`, issues)
    }
  })
  return issues
}

/** 建立 ID 索引，并为重复记录保留首次定义和稳定诊断路径。 */
function indexRecords<T extends { id: string }>(records: readonly T[], base: string, issues: ContentSnapshotIssue[]): Map<string, T> {
  const result = new Map<string, T>()
  records.forEach((record, index) => {
    if (result.has(record.id)) {
      addSemanticIssue(issues, `${base}/${index}/id`, 'duplicate-id')
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

/** 汇总快照中可作为内部跳转目标的全部已知内容 ID。 */
function collectKnownContentIds(
  snapshot: YujianContentSnapshot,
  indexes: HomepageIndexes,
  tracks: ReadonlyMap<string, Track>,
): Set<string> {
  return new Set([
    snapshot.artist.id,
    ...indexes.heroes.keys(),
    ...indexes.releases.keys(),
    ...tracks.keys(),
    ...indexes.videos.keys(),
    ...indexes.events.keys(),
    ...indexes.moments.keys(),
  ])
}

/**
 * 按板块启用状态、限制和时间窗口计算构建时实际可见的内容 ID。
 */
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

/** 校验各首页板块的条目 ID 是否属于对应内容集合。 */
function validateHomepage(snapshot: YujianContentSnapshot, indexes: HomepageIndexes, issues: ContentSnapshotIssue[]) {
  snapshot.homepage.sections.forEach((section, sectionIndex) => {
    const base = `/homepage/sections/${sectionIndex}/itemIds`
    if (section.type === 'artist') {
      section.itemIds.forEach((id, itemIndex) => {
        if (id !== snapshot.artist.id) addSemanticIssue(issues, `${base}/${itemIndex}`, 'missing-reference')
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

/** 校验大封面素材、关联作品和内部跳转目标。 */
function validateHeroes(
  records: readonly HeroSlide[],
  assets: ReadonlyMap<string, Asset>,
  releases: ReadonlyMap<string, Release>,
  knownContentIds: ReadonlySet<string>,
  renderedContentIds: ReadonlySet<string>,
  issues: ContentSnapshotIssue[],
) {
  records.forEach((record, index) => {
    const base = `/heroSlides/${index}`
    requireAssetKind(record.assetId, assets, [record.mediaKind], `${base}/assetId`, issues)
    if (record.mobileAssetId) requireAssetKind(record.mobileAssetId, assets, [record.mediaKind], `${base}/mobileAssetId`, issues)
    if (record.posterAssetId) requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `${base}/posterAssetId`, issues)
    if (record.releaseId) requireReference(record.releaseId, releases, `${base}/releaseId`, issues)
    validateInternalTarget(record.target, knownContentIds, renderedContentIds, `${base}/target/contentId`, issues)
  })
}

/** 校验作品封面、曲目引用及曲目归属关系。 */
function validateReleases(
  records: readonly Release[],
  assets: ReadonlyMap<string, Asset>,
  tracks: ReadonlyMap<string, Track>,
  issues: ContentSnapshotIssue[],
) {
  records.forEach((record, index) => {
    const base = `/releases/${index}`
    requireAssetKind(record.coverAssetId, assets, ['image', 'gif'], `${base}/coverAssetId`, issues)
    record.trackIds.forEach((trackId, trackIndex) => {
      requireReference(trackId, tracks, `${base}/trackIds/${trackIndex}`, issues)
      const track = tracks.get(trackId)
      if (track && track.releaseId !== record.id) addSemanticIssue(issues, `${base}/trackIds/${trackIndex}`, 'reference-mismatch')
    })
  })
}

/** 校验曲目的作品归属和可选试听音频素材。 */
function validateTracks(
  records: readonly Track[],
  assets: ReadonlyMap<string, Asset>,
  releases: ReadonlyMap<string, Release>,
  issues: ContentSnapshotIssue[],
) {
  records.forEach((record, index) => {
    const base = `/tracks/${index}`
    requireReference(record.releaseId, releases, `${base}/releaseId`, issues)
    if (record.previewAssetId) requireAssetKind(record.previewAssetId, assets, ['audio'], `${base}/previewAssetId`, issues)
  })
}

/** 校验视频海报与可选视频文件的素材类型。 */
function validateVideos(records: readonly Video[], assets: ReadonlyMap<string, Asset>, issues: ContentSnapshotIssue[]) {
  records.forEach((record, index) => {
    const base = `/videos/${index}`
    requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `${base}/posterAssetId`, issues)
    if (record.videoAssetId) requireAssetKind(record.videoAssetId, assets, ['video'], `${base}/videoAssetId`, issues)
  })
}

/** 校验现场活动的可选海报素材。 */
function validateEvents(records: readonly Event[], assets: ReadonlyMap<string, Asset>, issues: ContentSnapshotIssue[]) {
  records.forEach((record, index) => {
    if (record.posterAssetId) requireAssetKind(record.posterAssetId, assets, ['image', 'gif'], `/events/${index}/posterAssetId`, issues)
  })
}

/** 校验片段素材及其可选内部跳转目标。 */
function validateMoments(
  records: readonly Moment[],
  assets: ReadonlyMap<string, Asset>,
  knownContentIds: ReadonlySet<string>,
  renderedContentIds: ReadonlySet<string>,
  issues: ContentSnapshotIssue[],
) {
  records.forEach((record, index) => {
    requireAssetKind(record.assetId, assets, ['image', 'gif'], `/moments/${index}/assetId`, issues)
    validateInternalTarget(record.target, knownContentIds, renderedContentIds, `/moments/${index}/target/contentId`, issues)
  })
}

/** 区分不存在的内部目标与存在但未在首页渲染的目标。 */
function validateInternalTarget(
  target: HeroSlide['target'] | Moment['target'],
  knownContentIds: ReadonlySet<string>,
  renderedContentIds: ReadonlySet<string>,
  path: string,
  issues: ContentSnapshotIssue[],
) {
  if (target?.kind !== 'internal') return
  if (!knownContentIds.has(target.contentId)) {
    addSemanticIssue(issues, path, 'missing-reference')
  } else if (!renderedContentIds.has(target.contentId)) {
    addSemanticIssue(issues, path, 'hidden-target')
  }
}

/** 要求指定 ID 存在于目标记录索引中。 */
function requireReference(id: string, records: ReadonlyMap<string, unknown>, path: string, issues: ContentSnapshotIssue[]) {
  if (!records.has(id)) addSemanticIssue(issues, path, 'missing-reference')
}

/** 要求素材存在且属于调用方允许的素材类型。 */
function requireAssetKind(
  id: string,
  assets: ReadonlyMap<string, Asset>,
  allowedKinds: readonly Asset['kind'][],
  path: string,
  issues: ContentSnapshotIssue[],
) {
  const asset = assets.get(id)
  if (!asset) {
    addSemanticIssue(issues, path, 'missing-reference')
  } else if (!allowedKinds.includes(asset.kind)) {
    addSemanticIssue(issues, path, 'asset-kind')
  }
}

/** 以统一结构追加一个语义诊断。 */
function addSemanticIssue(issues: ContentSnapshotIssue[], path: string, code: string): void {
  issues.push({ path, source: 'semantic', code })
}

/** 按 RFC 6901 转义单个 JSON Pointer 路径段。 */
function escapePointer(value: string): string {
  return value.replaceAll('~', '~0').replaceAll('/', '~1')
}

/**
 * 为联合类型 Schema 注入 Ajv discriminator，减少无关分支错误噪声。
 */
function addSchemaDiscriminator(root: unknown, path: readonly string[], propertyName: string): void {
  let current = root
  for (const segment of path) {
    if (!isRecord(current)) throw new Error(`Invalid schema discriminator path: ${path.join('/')}`)
    current = current[segment]
  }
  if (!isRecord(current)) throw new Error(`Invalid schema discriminator target: ${path.join('/')}`)
  current.type = 'object'
  current.discriminator = { propertyName }
}

/** 判断未知值是否为可安全索引的非数组对象。 */
function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value)
}

/** 严格校验 YYYY-MM-DD，并拒绝日期自动进位。 */
function isISODate(value: string): boolean {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value)
  if (!match) return false
  const year = Number(match[1])
  const month = Number(match[2])
  const day = Number(match[3])
  const parsed = new Date(Date.UTC(year, month - 1, day))
  return parsed.getUTCFullYear() === year && parsed.getUTCMonth() === month - 1 && parsed.getUTCDate() === day
}

/** 校验包含明确时区且能被解析的 RFC 3339 日期时间。 */
function isRFC3339DateTime(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value)
    && !Number.isNaN(Date.parse(value))
}

/**
 * 校验安全 HTTPS 外链，拒绝凭据、反斜杠、空主机和非法端口。
 */
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
