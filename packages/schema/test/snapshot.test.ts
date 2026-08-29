import { createHash } from 'node:crypto'
import { existsSync, readFileSync } from 'node:fs'
import { extname } from 'node:path'
import { fileURLToPath } from 'node:url'
import Ajv2020 from 'ajv/dist/2020.js'
import sharp from 'sharp'
import { describe, expect, it } from 'vitest'
import schema from '../schema/content-snapshot.schema.json'

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
