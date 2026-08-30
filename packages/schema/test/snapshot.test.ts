import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { extname } from 'node:path'
import { fileURLToPath } from 'node:url'
import Ajv2020 from 'ajv/dist/2020.js'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'
import schema from '../schema/content-snapshot.schema.json'
import { validateContentSnapshot } from '../src/validate'

const fixture = JSON.parse(
  readFileSync(new URL('../../../content/fixtures/homepage.json', import.meta.url), 'utf8'),
)
const extensionMimeTypes: Readonly<Record<string, string>> = {
  '.wav': 'audio/wav',
  '.webp': 'image/webp',
}

describe('首页快照契约', () => {
  const validate = new Ajv2020({ allErrors: true, validateFormats: false }).compile(schema)

  it('接受完整双语 fixture', () => {
    expect(validate(fixture), JSON.stringify(validate.errors)).toBe(true)
  })

  it('生成类型保留本地媒体 URL 形式', () => {
    const generated = readFileSync(new URL('../src/generated.ts', import.meta.url), 'utf8')
    expect(generated).toMatch(/export type MediaUrl = HttpsUrl \| `\/media\/\$\{string\}`;/)
  })

  it('拒绝没有版本号的快照', () => {
    const invalid = structuredClone(fixture)
    delete invalid.schemaVersion
    expect(validate(invalid)).toBe(false)
  })

  it('拒绝非 HTTPS 的外部平台链接', () => {
    const invalid = structuredClone(fixture)
    invalid.site.socialLinks[0].url = 'http://example.com'
    expect(validate(invalid)).toBe(false)
  })

  it('允许音乐板块配置安全的全部作品入口', () => {
    const configured = structuredClone(fixture)
    const musicSection = configured.homepage.sections.find(
      (section: { type: string }) => section.type === 'music',
    )
    musicSection.moreLink = {
      provider: 'qq-music',
      url: 'https://y.qq.com/n/ryqq_v2/singer/0036zydh4H05PB',
    }

    expect(validate(configured), JSON.stringify(validate.errors)).toBe(true)

    musicSection.moreLink.url = 'http://example.com/music'
    expect(validate(configured)).toBe(false)
  })

  it('拒绝素材 kind 与 MIME 不一致', () => {
    const invalid = structuredClone(fixture)
    invalid.assets[0].kind = 'audio'
    invalid.assets[0].mimeType = 'image/webp'

    expect(validate(invalid)).toBe(false)
  })

  it('拒绝没有有效主机的 HTTPS URL', () => {
    for (const url of [
      'https://',
      'https://?q=x',
      'https:///assets/cover.webp',
      'https:// /cover.webp',
      'https://:443/path',
      'https://@/path',
      'https://%/path',
      'https://[::1/path',
      'https://example.com:0/path',
      'https://example.com:65536/path',
      'https://@example.com/path',
      'https://user@example.com/path',
      'https://user:pass@example.com/path',
    ]) {
      const invalid = structuredClone(fixture)
      invalid.site.socialLinks[0].url = url
      expect(validateContentSnapshot(invalid), url).toContain('/site/socialLinks/0/url')
    }
  })

  it('拒绝把非音频素材用作试听', () => {
    const invalid = structuredClone(fixture)
    const trackIndex = invalid.tracks.findIndex((track: { previewAssetId?: string }) => track.previewAssetId)
    const previewId = invalid.tracks[trackIndex].previewAssetId
    const asset = invalid.assets.find((candidate: { id: string }) => candidate.id === previewId)
    asset.kind = 'image'
    asset.mimeType = 'image/webp'

    expect(validateContentSnapshot(invalid)).toContain(`/tracks/${trackIndex}/previewAssetId`)
  })

  it('拒绝把音频素材用作封面', () => {
    const invalid = structuredClone(fixture)
    const audioAsset = invalid.assets.find((asset: { kind: string }) => asset.kind === 'audio')
    invalid.releases[0].coverAssetId = audioAsset.id

    expect(validateContentSnapshot(invalid)).toContain('/releases/0/coverAssetId')
  })

  it('拒绝指向被板块 limit 截断内容的内部目标', () => {
    const invalid = structuredClone(fixture)
    const hero = invalid.heroSlides.find((slide: { id: string }) => slide.id === 'hero_release')
    const musicSection = invalid.homepage.sections.find(
      (section: { type: string }) => section.type === 'music',
    )
    hero.target.contentId = 'release_02'
    musicSection.limit = 1

    expect(validateContentSnapshot(invalid)).toContain('/heroSlides/2/target/contentId')
  })

  it('拒绝指向禁用板块内容的内部目标', () => {
    const invalid = structuredClone(fixture)
    const hero = invalid.heroSlides.find((slide: { id: string }) => slide.id === 'hero_release')
    const artistSection = invalid.homepage.sections.find(
      (section: { type: string }) => section.type === 'artist',
    )
    hero.target.contentId = 'artist_primary'
    artistSection.enabled = false

    expect(validateContentSnapshot(invalid)).toContain('/heroSlides/2/target/contentId')
  })

  it('拒绝指向未编排内容的内部目标', () => {
    const invalid = structuredClone(fixture)
    const musicSection = invalid.homepage.sections.find(
      (section: { type: string }) => section.type === 'music',
    )
    musicSection.itemIds = musicSection.itemIds.filter((id: string) => id !== 'release_01')

    expect(validateContentSnapshot(invalid)).toContain('/heroSlides/2/target/contentId')
  })

  it('拒绝指向已过展示期内容的内部目标', () => {
    const invalid = structuredClone(fixture)
    const hero = invalid.heroSlides.find((slide: { id: string }) => slide.id === 'hero_release')
    hero.target.contentId = 'event_01'
    invalid.events[0].dateTime = invalid.generatedAt

    expect(validateContentSnapshot(invalid)).toContain('/heroSlides/2/target/contentId')
  })

  it('所有本地资源引用都指向格式、尺寸和校验和匹配的文件', async () => {
    for (const asset of fixture.assets) {
      if (!asset.src.startsWith('/')) {
        continue
      }

      const file = new URL(`../../../apps/web/public${asset.src}`, import.meta.url)
      expect(existsSync(file), `${asset.id}: ${asset.src}`).toBe(true)

      const filePath = fileURLToPath(file)
      const buffer = readFileSync(filePath)
      const checksum = createHash('sha256').update(buffer).digest('hex')
      expect(asset.byteSize, `${asset.id}: byteSize`).toBe(buffer.length)
      expect(asset.checksum, `${asset.id}: checksum`).toBe(`sha256:${checksum}`)

      expect(asset.mimeType, `${asset.id}: mimeType`).toBe(
        extensionMimeTypes[extname(filePath)],
      )

      if (asset.mimeType === 'image/webp') {
        const metadata = await sharp(buffer).metadata()
        expect(metadata.format, `${asset.id}: format`).toBe('webp')
        expect(metadata.width, `${asset.id}: width`).toBe(asset.width)
        expect(metadata.height, `${asset.id}: height`).toBe(asset.height)
      }

      if (asset.mimeType === 'audio/wav') {
        expect(buffer.toString('ascii', 0, 4), `${asset.id}: RIFF`).toBe('RIFF')
        expect(buffer.toString('ascii', 8, 12), `${asset.id}: WAVE`).toBe('WAVE')
        const sampleRate = buffer.readUInt32LE(24)
        const byteRate = buffer.readUInt32LE(28)
        const dataSize = buffer.readUInt32LE(40)
        expect(dataSize / byteRate, `${asset.id}: duration`).toBeCloseTo(
          asset.durationSeconds,
        )
        expect(sampleRate, `${asset.id}: sampleRate`).toBe(44_100)
      }
    }
  })
})
