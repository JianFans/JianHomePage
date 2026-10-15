import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import { test } from 'node:test'
import { createStore } from '../src/store.mjs'

/** Model SQL transport failures only; actual SQL constraints are covered against PostgreSQL. */
function database({ locked = true, unlockError = false } = {}) {
  const client = new EventEmitter()
  const calls = []
  client.query = async (sql, args) => {
    calls.push([sql, args])
    if (sql.includes('pg_try_advisory_lock')) return { rows: [{ locked }] }
    if (sql.includes('pg_advisory_unlock') && unlockError) throw new Error('disconnected')
    return { rows: [], rowCount: 0 }
  }
  client.release = damaged => calls.push(['release', damaged])
  return { client, calls, query: client.query, async connect() { return client } }
}

test('lock loss blocks later writes and evicts the damaged database connection', async () => {
  const db = database()
  let losses = 0
  const store = createStore(db, { onLockLost: () => { losses++ } })
  await store.withLock(async locked => {
    db.client.emit('error', new Error('connection lost'))
    await assert.rejects(locked.update('id', 'building', { phase: 'submitting' }), /lock lost/)
  })
  assert.equal(losses, 1)
  assert.deepEqual(db.calls.at(-1), ['release', true])
  assert.ok(!db.calls.some(([sql]) => sql.startsWith('UPDATE')))
})

test('busy workers return immediately and failed unlocks notify the owner', async () => {
  const busy = database({ locked: false })
  assert.equal(await createStore(busy).withLock(() => assert.fail('busy lock entered')), null)
  assert.deepEqual(busy.calls.at(-1), ['release', false])
  let lost = false
  const broken = database({ unlockError: true })
  await createStore(broken, { onLockLost: () => { lost = true } }).withLock(async () => {})
  assert.ok(lost)
  assert.deepEqual(broken.calls.at(-1), ['release', true])
})

test('SQL failures remain failures; missing records and invalid update identifiers are bounded', async () => {
  const db = database()
  const store = createStore(db)
  await store.health()
  assert.equal(await store.get('missing'), null)
  assert.equal(await store.active(), null)
  await assert.rejects(store.update('id', 'queued', {}), /Invalid/)
  await assert.rejects(store.update('id', 'queued', { 'phase;DROP': 'failed' }), /Invalid/)
  await assert.rejects(store.update('id', 'queued', { phase: 'failed' }), { name: 'ConflictError' })
  const failing = createStore({ async query() { throw new Error('offline') } })
  await assert.rejects(failing.enqueue('key', {}, {}), /offline/)
})

test('same keys preserve saved targets and a changed request conflicts', async () => {
  const row = { id: 'build_id', request: { releaseId: 'rel_test' }, target: { projectId: 'old' }, phase: 'queued', reconciliation: [] }
  const saved = createStore({ async query() { return { rows: [row], rowCount: 1 } } })
  const job = await saved.enqueue('key', row.request, { projectId: 'new' })
  assert.equal(job.target.projectId, 'old')
  await assert.rejects(saved.enqueue('key', { releaseId: 'rel_other' }, {}), { name: 'ConflictError' })
  const updated = await saved.update('build_id', 'queued', { phase: 'building', reconciliation: [] })
  assert.equal(updated.id, row.id)
  let insertion = true
  const conflict = createStore({ async query() {
    if (insertion) { insertion = false; const error = new Error('unique'); error.code = '23505'; throw error }
    return { rows: [row] }
  } })
  assert.equal((await conflict.enqueue('key', row.request, {})).id, row.id)
})
