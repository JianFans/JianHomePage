import assert from 'node:assert/strict'
import { Readable } from 'node:stream'
import { test } from 'node:test'

const module = await import('../src/snapshot-reader.mjs').catch(() => ({}))

test('S3 reader forwards cancellation and closes the response body when abandoned', async () => {
  assert.equal(typeof module.createSnapshotReader, 'function', 'snapshot reader must exist')
  let options
  let request
  const body = Readable.from([Buffer.from('snapshot')])
  const read = module.createSnapshotReader({ async send(command, config) { request = command.input; options = config; return { Body: body } } }, 'snapshots')
  const controller = new AbortController()
  const stream = read('snapshots/key', { signal: controller.signal })
  assert.equal(Buffer.from((await stream.next()).value).toString(), 'snapshot')
  assert.deepEqual(request, { Bucket: 'snapshots', Key: 'snapshots/key' })
  assert.equal(options.abortSignal, controller.signal)
  await stream.return()
  assert.ok(body.destroyed)
})

test('aborting a stalled S3 response destroys it without exposing storage internals', async () => {
  const body = new Readable({ read() {} })
  const read = module.createSnapshotReader({ async send() { return { Body: body } } }, 'snapshots')
  const controller = new AbortController()
  const next = read('key', { signal: controller.signal }).next()
  await new Promise(resolve => setImmediate(resolve))
  controller.abort()
  await assert.rejects(next, /Snapshot read/)
  assert.ok(body.destroyed)
})
