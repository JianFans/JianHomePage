import assert from 'node:assert/strict'
import { once } from 'node:events'
import { test } from 'node:test'
import { readFile } from 'node:fs/promises'
import { request as httpRequest } from 'node:http'

const module = await import('../src/http.mjs').catch(() => ({}))
const token = 'internal-token'
const body = { releaseId: 'rel_test', snapshotChecksum: `sha256:${'a'.repeat(64)}`, snapshotKey: `snapshots/rel_test/sha256:${'a'.repeat(64)}.json` }

/** Exercise an actual HTTP server so auth, parsing and response contracts are verified together. */
async function serverContext(t, overrides = {}) {
  const jobs = new Map()
  const store = {
    async health() {},
    async enqueue(key, request, target) {
      const old = jobs.get(key)
      if (old && JSON.stringify(old.request) !== JSON.stringify(request)) { const error = new Error(); error.name = 'ConflictError'; throw error }
      const job = old || { id: `build_${'a'.repeat(64)}`, phase: 'queued', request, target }
      jobs.set(key, job)
      return job
    },
    async get(id) { return [...jobs.values()].find(job => job.id === id) },
  }
  const server = module.createGateway?.({ store, token, target: {}, reconcile: async () => { throw new Error('private detail') }, ...overrides })
  assert.ok(server, 'publisher gateway must exist')
  t.after(() => new Promise(resolve => { server.closeAllConnections(); server.close(resolve) }))
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const origin = `http://127.0.0.1:${server.address().port}`
  return {
    origin,
    jobs,
    request(path, method = 'GET', data, headers = {}) {
      return fetch(`${origin}${path}`, { method, headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', ...headers }, ...(data === undefined ? {} : { body: typeof data === 'string' ? data : JSON.stringify(data) }) })
    },
  }
}

test('accepts the Go contract once and returns the durable build through its status endpoint', async t => {
  assert.equal(typeof module.createGateway, 'function')
  const context = await serverContext(t)
  const headers = { 'Idempotency-Key': 'publish-key' }
  const response = await context.request('/builds', 'POST', body, headers)
  assert.equal(response.status, 202)
  assert.equal(response.headers.get('cache-control'), 'no-store')
  const build = await response.json()
  assert.equal(build.status, 'pending')
  const repeat = await context.request('/builds', 'POST', body, headers)
  assert.equal((await repeat.json()).id, build.id)
  assert.equal(context.jobs.size, 1)
  const status = await context.request(`/builds/${build.id}`)
  assert.equal((await status.json()).id, build.id)
  assert.equal((await context.request(`/builds/build_${'b'.repeat(64)}`)).status, 404)
})

test('normalizes UTF-8 header bytes like Go while preserving opaque key identity', async t => {
  const context = await serverContext(t)
  for (const [input, expected, status] of [
    ['12345678', '12345678', 202],
    ['x'.repeat(128), 'x'.repeat(128), 202],
    ['\u00e9'.repeat(4), '\u00e9'.repeat(4), 202],
    ['\u00e9'.repeat(64), '\u00e9'.repeat(64), 202],
    ['\u0085\u00a0 publish-key \u3000', 'publish-key', 202],
    ['\ufeffpublish-key', '\ufeffpublish-key', 202],
    ['1234567', undefined, 400],
    ['\u00e9'.repeat(65), undefined, 400],
    ['publish\tkey', undefined, 400],
    ['publish\u0085key', undefined, 400],
    [Buffer.from([112, 117, 98, 108, 105, 115, 104, 255, 107, 101, 121]), undefined, 400],
  ]) {
    // A Buffer body keeps node:http header serialization in Latin-1, matching Go's UTF-8 wire bytes.
    const wireKey = Buffer.from(input).toString('latin1')
    const size = context.jobs.size
    const actual = await new Promise((resolve, reject) => {
      const request = httpRequest(`${context.origin}/builds`, {
        method: 'POST', headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json', 'Idempotency-Key': wireKey },
      }, response => {
        response.resume()
        response.on('end', () => resolve(response.statusCode))
      })
      request.on('error', reject)
      request.end(Buffer.from(JSON.stringify(body)))
    })
    assert.equal(actual, status, `unexpected status for ${JSON.stringify(input)}`)
    if (expected !== undefined) assert.ok(context.jobs.has(expected), `the durable key must match Go normalization for ${JSON.stringify(input)}`)
    else assert.equal(context.jobs.size, size, 'invalid keys must not create jobs')
  }
})

test('all operational endpoints require a Bearer token and errors never expose internals', async t => {
  const context = await serverContext(t)
  for (const path of ['/builds', `/builds/build_${'a'.repeat(64)}`, `/builds/build_${'a'.repeat(64)}/reconcile`]) {
    const response = await context.request(path, path === '/builds' || path.endsWith('reconcile') ? 'POST' : 'GET', undefined, { Authorization: 'Bearer wrong' })
    assert.equal(response.status, 401)
    assert.equal(response.headers.get('access-control-allow-origin'), null)
  }
  assert.equal((await context.request('/healthz')).status, 200)
  const failure = await context.request(`/builds/build_${'a'.repeat(64)}/reconcile`, 'POST', { confirmedAbsent: true, note: 'checked' })
  assert.equal(failure.status, 503)
  assert.doesNotMatch(await failure.text(), /private/)
})

test('accepts canonical release IDs beyond 240 characters within the bounded request body', async t => {
  const schema = JSON.parse(await readFile(new URL('../../../packages/schema/schema/content-snapshot.schema.json', import.meta.url), 'utf8'))
  const releaseId = `rel_${'a'.repeat(241)}`
  assert.match(releaseId, new RegExp(schema.properties.releaseId.pattern))
  assert.equal(schema.properties.releaseId.maxLength, undefined)
  const context = await serverContext(t)
  const request = { ...body, releaseId, snapshotKey: `snapshots/${releaseId}/${body.snapshotChecksum}.json` }
  const response = await context.request('/builds', 'POST', request, { 'Idempotency-Key': 'long-release-key' })
  assert.equal(response.status, 202)
  assert.deepEqual(context.jobs.get('long-release-key').request, request)
})

test('rejects malformed, oversized and ambiguous requests before creating a job', async t => {
  const context = await serverContext(t)
  const headers = { 'Idempotency-Key': 'publish-key' }
  for (const invalid of [
    '{', [], { ...body, unknown: true }, { ...body, snapshotKey: '../secret' },
    { ...body, releaseId: 'rel_other' }, { ...body, snapshotChecksum: 'bad' },
  ]) {
    assert.equal((await context.request('/builds', 'POST', invalid, headers)).status, 400)
  }
  assert.equal((await context.request('/builds', 'POST', body)).status, 400)
  assert.equal((await context.request('/builds', 'POST', body, { ...headers, 'Content-Type': 'text/plain' })).status, 415)
  assert.equal((await context.request('/builds', 'POST', 'x'.repeat(17000), headers)).status, 413)
  assert.equal(context.jobs.size, 0)
})

test('distinguishes conflicts, unsupported methods, missing paths and unavailable persistence', async t => {
  const context = await serverContext(t)
  const headers = { 'Idempotency-Key': 'publish-key' }
  await context.request('/builds', 'POST', body, headers)
  const changed = { ...body, releaseId: 'rel_changed', snapshotKey: body.snapshotKey.replace('rel_test', 'rel_changed') }
  assert.equal((await context.request('/builds', 'POST', changed, headers)).status, 409)
  assert.equal((await context.request('/builds', 'DELETE')).status, 405)
  assert.equal((await context.request('/other')).status, 404)
  const down = await serverContext(t, { store: { async health() { throw new Error('database password') } } })
  const health = await down.request('/healthz')
  assert.equal(health.status, 503)
  assert.doesNotMatch(await health.text(), /password/)
})
