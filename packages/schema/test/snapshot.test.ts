import { readFileSync } from 'node:fs'
import Ajv2020 from 'ajv/dist/2020.js'
import { describe, expect, it } from 'vitest'
import schema from '../schema/content-snapshot.schema.json'

const fixture = JSON.parse(
  readFileSync(new URL('../../../content/fixtures/homepage.json', import.meta.url), 'utf8'),
)

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
})
