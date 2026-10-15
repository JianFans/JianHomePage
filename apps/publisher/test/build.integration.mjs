import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { access, mkdtemp, readFile, readdir, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { isAbsolute, join } from 'node:path'
import test from 'node:test'
import { createBuilder } from '../src/builder.mjs'

test('installed runtime builds verified static output without Corepack or cloud credentials', { timeout: 660000 }, async t => {
  // Only point this at a disposable image or checkout: the builder replaces its Nuxt output.
  const sourceRoot = process.env.YUJIAN_TEST_BUILD_SOURCE
  assert.ok(sourceRoot && isAbsolute(sourceRoot), 'An explicit isolated build source is required')
  const workRoot = await mkdtemp(join(tmpdir(), 'publisher-build-integration-'))
  t.after(() => rm(workRoot, { recursive: true, force: true }))
  const snapshot = JSON.parse(await readFile(join(sourceRoot, 'content/fixtures/homepage.json'), 'utf8'))
  snapshot.releaseId = 'rel_publisher_build_integration'
  // This exercises build mechanics with test-only URLs, never approved production content.
  for (const asset of snapshot.assets) {
    asset.src = `https://media.example.com/validation/${asset.id}${asset.src.slice(asset.src.lastIndexOf('.'))}`
  }
  const bytes = Buffer.from(JSON.stringify(snapshot))
  const snapshotChecksum = `sha256:${createHash('sha256').update(bytes).digest('hex')}`
  const build = createBuilder({ sourceRoot, workRoot, async *readSnapshot() { yield bytes } })
  const directory = await build({
    id: `build_${'d'.repeat(64)}`, releaseId: snapshot.releaseId, snapshotChecksum,
    snapshotKey: `snapshots/${snapshot.releaseId}/${snapshotChecksum}.json`,
  })
  assert.match(await readFile(join(directory, 'index.html'), 'utf8'), /<html/)
  await assert.rejects(access(join(directory, 'media/hero-studio.webp')))
  assert.deepEqual(await readdir(workRoot), [])
})
