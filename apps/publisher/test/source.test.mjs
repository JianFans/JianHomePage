import assert from 'node:assert/strict'
import { mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'

const module = await import('../src/source.mjs').catch(() => ({}))

test('source revision must match the immutable image before any build is accepted', async t => {
  assert.equal(typeof module.verifySourceRevision, 'function', 'source verification must exist')
  const root = await mkdtemp(join(tmpdir(), 'publisher-revision-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  await writeFile(join(root, '.publisher-revision'), 'a'.repeat(40))
  await module.verifySourceRevision(root, 'a'.repeat(40))
  await assert.rejects(module.verifySourceRevision(root, 'b'.repeat(40)), /revision/)
  await writeFile(join(root, '.publisher-revision'), 'main')
  await assert.rejects(module.verifySourceRevision(root, 'a'.repeat(40)), /revision/)
})
