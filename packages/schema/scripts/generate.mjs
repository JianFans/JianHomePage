import { mkdir, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { compileFromFile } from 'json-schema-to-typescript'

const schemaPath = fileURLToPath(
  new URL('../schema/content-snapshot.schema.json', import.meta.url),
)
const outputPath = new URL('../src/generated.ts', import.meta.url)

const output = await compileFromFile(schemaPath, {
  bannerComment: '/* 此文件由 pnpm schema:generate 生成，请勿手工修改。 */',
})

await mkdir(new URL('../src', import.meta.url), { recursive: true })
await writeFile(outputPath, output)
