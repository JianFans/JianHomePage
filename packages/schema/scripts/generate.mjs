import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { compileFromFile } from 'json-schema-to-typescript'

const schemaPath = fileURLToPath(
  new URL('../schema/content-snapshot.schema.json', import.meta.url),
)
const outputPath = new URL('../src/generated.ts', import.meta.url)

const generated = await compileFromFile(schemaPath, {
  bannerComment: '/* 此文件由 pnpm schema:generate 生成，请勿手工修改。 */',
})

// json-schema-to-typescript collapses the MediaUrl oneOf into HttpsUrl. Keep
// the local immutable asset path explicitly typed so callers can distinguish
// build-local media from external HTTPS links.
const output = generated.replace(
  'export type MediaUrl = HttpsUrl;',
  'export type MediaUrl = HttpsUrl | `/media/${string}`;',
)
if (output === generated) {
  throw new Error('MediaUrl type was not emitted in the expected form')
}

await mkdir(new URL('../src', import.meta.url), { recursive: true })
await writeFile(outputPath, output)
