import assert from 'node:assert/strict'
import { randomBytes } from 'node:crypto'
import { mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { spawn } from 'node:child_process'
import { createServer } from 'node:net'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { setTimeout as delay } from 'node:timers/promises'
import { test } from 'node:test'
import pg from 'pg'
import { createWorker } from '../src/worker.mjs'
import { createGateway } from '../src/http.mjs'
import { once } from 'node:events'

const module = await import('../src/store.mjs').catch(() => ({}))
const databaseURL = process.env.YUJIAN_TEST_POSTGRES_URL
const target = { projectId: 'project', region: 'china', codeRevision: 'a'.repeat(40), publicUrl: 'https://yujian.me/' }
const request = { releaseId: 'rel_test', snapshotKey: `snapshots/rel_test/sha256:${'b'.repeat(64)}.json`, snapshotChecksum: `sha256:${'b'.repeat(64)}` }

test('PostgreSQL releases an acquired lock when its response becomes uncertain', { skip: !databaseURL }, async t => {
  const pool = new pg.Pool({ connectionString: databaseURL, max: 1 })
  const observer = new pg.Client({ connectionString: databaseURL })
  t.after(async () => { await observer.end(); await pool.end() })
  await observer.connect()
  const client = await pool.connect()
  const ended = once(client, 'end')
  const query = client.query.bind(client)
  const timeout = new Error('Query read timeout')
  client.query = async (...args) => {
    const result = await query(...args)
    // The backend owns the lock, but pg's read timeout can discard its response.
    if (args[0].includes('pg_try_advisory_lock')) throw timeout
    return result
  }
  await assert.rejects(module.createStore({ connect: async () => client }).withLock(() => assert.fail('uncertain lock entered')), timeout)
  assert.equal(pool.totalCount, 0, 'the uncertain session must be removed from the pool')
  await ended
  const result = await observer.query('SELECT pg_try_advisory_lock($1) AS locked', [865734220])
  assert.equal(result.rows[0].locked, true, 'an independent session must be able to acquire the released lock')
})

test('PostgreSQL persists idempotency, active slots, submission recovery and exclusive worker locks', { skip: !databaseURL }, async t => {
  assert.equal(typeof module.createStore, 'function', 'durable publisher store must exist')
  const admin = new pg.Pool({ connectionString: databaseURL })
  const schema = `publisher_test_${randomBytes(8).toString('hex')}`
  await admin.query(`CREATE SCHEMA ${schema}`)
  const pool = new pg.Pool({ connectionString: databaseURL, options: `-c search_path=${schema}`, max: 4 })
  t.after(async () => { await pool.end(); await admin.query(`DROP SCHEMA ${schema} CASCADE`); await admin.end() })
  await pool.query('CREATE TABLE schema_migrations(version TEXT PRIMARY KEY)')
  const migration = await readFile(new URL('../../server/internal/store/postgres/migrations/0008_publisher_builds.sql', import.meta.url), 'utf8')
  await pool.query(migration)
  await pool.query(migration)
  const store = module.createStore(pool)
  await store.health()
  const [one, two] = await Promise.all([store.enqueue('publish-key', request, target), store.enqueue('publish-key', request, target)])
  assert.equal(one.id, two.id)
  assert.deepEqual(one.request, request)
  await assert.rejects(store.enqueue('publish-key', { ...request, releaseId: 'rel_other' }, target), { name: 'ConflictError' })
  await assert.rejects(store.enqueue('second-key', request, target), { name: 'ConflictError' })
  await assert.rejects(store.update(one.id, 'building', { phase: 'failed' }), { name: 'ConflictError' })
  await store.update(one.id, 'queued', { phase: 'building' })
  await store.update(one.id, 'building', { phase: 'submitting' })
  const restarted = module.createStore(pool)
  assert.equal((await restarted.active()).phase, 'submitting')
  assert.deepEqual((await restarted.get(one.id)).target, target)
  let held
  let release
  const acquired = new Promise(resolve => { held = resolve })
  const keep = new Promise(resolve => { release = resolve })
  const lock = store.withLock(async locked => { assert.equal((await locked.active()).id, one.id); held(); await keep; return 'done' })
  await acquired
  assert.equal(await restarted.withLock(async () => assert.fail('second worker acquired the lock')), null)
  release()
  assert.equal(await lock, 'done')
  await assert.rejects(store.withLock(async () => { throw new Error('worker failure') }), /worker failure/)
  assert.equal(await restarted.withLock(async () => 'released'), 'released')
  await store.update(one.id, 'submitting', { phase: 'deployed', deploymentId: 'deployment', reconciliation: [{ note: 'verified' }] })
  await store.update(one.id, 'deployed', { phase: 'succeeded', previewUrl: target.publicUrl })
  assert.equal(await store.active(), null)
  const next = await store.enqueue('second-key', request, target)
  assert.notEqual(next.id, one.id)
  // Retry remains bound to the original target even after a configuration change.
  assert.equal((await store.enqueue('publish-key', request, { ...target, projectId: 'other' })).target.projectId, 'project')
  // Exercise actual HTTP -> durable repository -> worker -> recovered status in one run.
  await store.update(next.id, 'queued', { phase: 'failed' })
  const gateway = createGateway({ store, token: 'integration-token', target })
  gateway.listen(0, '127.0.0.1')
  await once(gateway, 'listening')
  t.after(() => new Promise(resolve => { gateway.closeAllConnections(); gateway.close(resolve) }))
  const base = `http://127.0.0.1:${gateway.address().port}`
  const headers = { Authorization: 'Bearer integration-token', 'Content-Type': 'application/json', 'Idempotency-Key': 'http-publish-key' }
  const response = await fetch(`${base}/builds`, { method: 'POST', headers, body: JSON.stringify(request) })
  assert.equal(response.status, 202)
  const submitted = await response.json()
  let builds = 0
  let uploads = 0
  const options = {
    target,
    async build(job) { assert.equal(job.snapshotChecksum, request.snapshotChecksum); builds++; return '/artifact' },
    pages: {
      async deploy() { uploads++; return { deploymentId: 'http-deployment', projectId: 'project', env: 'Production' } },
      async get() { return { deploymentId: 'http-deployment', projectId: 'project', env: 'Production', status: 'succeeded' } },
    },
  }
  await store.withLock(locked => createWorker({ ...options, store: locked }).tick())
  await restarted.withLock(locked => createWorker({ ...options, store: locked }).tick())
  const completed = await (await fetch(`${base}/builds/${submitted.id}`, { headers })).json()
  assert.equal(completed.status, 'succeeded')
  assert.equal(builds, 1)
  assert.equal(uploads, 1)
  await verifyEntrypoint(databaseURL, schema)
})

/** Start the actual process with test-only credentials and verify readiness without any cloud writes. */
async function verifyEntrypoint(connectionString, schema) {
  const source = await mkdtemp(join(tmpdir(), 'publisher-boot-'))
  await writeFile(join(source, '.publisher-revision'), target.codeRevision)
  const addressProbe = createServer()
  addressProbe.listen(0, '127.0.0.1')
  await once(addressProbe, 'listening')
  const port = addressProbe.address().port
  await new Promise(resolve => addressProbe.close(resolve))
  const url = new URL(connectionString)
  url.searchParams.set('options', `-c search_path=${schema}`)
  const child = spawn(process.execPath, [fileURLToPath(new URL('../src/main.mjs', import.meta.url))], {
    windowsHide: true,
    env: {
      ...process.env,
      PUBLISHER_DATABASE_URL: url.href, PUBLISHER_TOKEN: 'gateway-test-token-'.repeat(3),
      PAGES_API_TOKEN: 'cloud-test-token', PAGES_PROJECT_ID: target.projectId, PAGES_REGION: target.region,
      PAGES_PUBLIC_URL: target.publicUrl, PUBLISHER_CODE_REVISION: target.codeRevision,
      PUBLISHER_SOURCE_ROOT: source, PUBLISHER_PORT: String(port), PUBLISHER_HOST: '127.0.0.1',
      S3_ENDPOINT: 'https://storage.example.com', S3_REGION: 'test-region', S3_BUCKET: 'test-bucket',
      S3_ACCESS_KEY_ID: 'test-id', S3_SECRET_ACCESS_KEY: 'test-secret',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })
  let output = ''
  child.stdout.on('data', chunk => { output += chunk })
  child.stderr.on('data', chunk => { output += chunk })
  const closed = once(child, 'close')
  try {
    let ready = false
    for (let attempt = 0; attempt < 100; attempt++) {
      assert.equal(child.exitCode, null, 'Publisher exited before readiness')
      try { ready = (await fetch(`http://127.0.0.1:${port}/healthz`, { signal: AbortSignal.timeout(500) })).status === 200 } catch { /* Startup may still be loading modules. */ }
      if (ready) break
      await delay(100)
    }
    assert.ok(ready, 'Publisher entrypoint did not become ready')
    assert.doesNotMatch(output, /test-secret|cloud-test-token|gateway-test-token/)
  } finally {
    child.kill('SIGTERM')
    await closed
    await rm(source, { recursive: true, force: true })
  }
}
