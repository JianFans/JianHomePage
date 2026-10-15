import assert from 'node:assert/strict'
import { test } from 'node:test'

const module = await import('../src/worker.mjs').catch(() => ({}))

const target = { projectId: 'project', region: 'china', codeRevision: 'a'.repeat(40), publicUrl: 'https://yujian.me/' }

/** Keep the real state machine independent of PostgreSQL and cloud availability. */
function setup(overrides = {}) {
  let job = { id: `build_${'b'.repeat(64)}`, phase: 'queued', target, request: { releaseId: 'rel_test' }, deploymentId: null, error: '', previewUrl: '', reconciliation: [] }
  const events = []
  const store = {
    async active() { return structuredClone(job) },
    async get() { return structuredClone(job) },
    async update(id, phase, fields) {
      assert.equal(id, job.id)
      assert.equal(phase, job.phase)
      events.push(fields.phase ?? phase)
      job = { ...job, ...fields }
      return structuredClone(job)
    },
  }
  const pages = {
    async deploy() { events.push('upload'); return { deploymentId: 'deployment', projectId: 'project', env: 'Production' } },
    async get() { return { deploymentId: 'deployment', projectId: 'project', env: 'Production', status: 'succeeded' } },
    ...overrides.pages,
  }
  const worker = module.createWorker?.({ store, target, build: async () => { events.push('build'); return '/artifact' }, ...overrides, pages })
  return { worker, events, get job() { return job }, set job(value) { job = value } }
}

test('worker persists the submission boundary before uploading and polls a known deployment', async () => {
  assert.equal(typeof module.createWorker, 'function', 'publishing worker must exist')
  const state = setup()
  await state.worker.tick()
  assert.deepEqual(state.events, ['building', 'build', 'submitting', 'upload', 'deployed'])
  assert.equal(state.job.deploymentId, 'deployment')
  await state.worker.tick()
  assert.equal(state.job.phase, 'succeeded')
  assert.equal(state.job.previewUrl, target.publicUrl)
  assert.equal(state.events.filter(item => item === 'upload').length, 1)
})

test('a crash or timeout while submitting never blindly creates a second deployment', async () => {
  const state = setup({ pages: { async deploy() { throw new Error('sensitive provider error') } } })
  await state.worker.tick()
  assert.equal(state.job.phase, 'submitting')
  assert.match(state.job.error, /reconciliation/i)
  assert.doesNotMatch(state.job.error, /sensitive/)
  await state.worker.tick()
  assert.equal(state.events.filter(item => item === 'build').length, 1)
  const restarted = setup()
  restarted.job = { ...state.job, error: '' }
  await restarted.worker.tick()
  assert.match(restarted.job.error, /reconciliation/i)
  assert.ok(!restarted.events.includes('upload'))
})

test('build failure is terminal but a provider status outage keeps the active task', async () => {
  const failed = setup({ async build() { throw new Error('secret snapshot') } })
  await failed.worker.tick()
  assert.equal(failed.job.phase, 'failed')
  assert.ok(!failed.events.includes('upload'))
  assert.doesNotMatch(failed.job.error, /secret/)
  const outage = setup({ pages: { async get() { throw new Error('token') } } })
  await outage.worker.tick()
  await outage.worker.tick()
  assert.equal(outage.job.phase, 'deployed')
  assert.doesNotMatch(outage.job.error, /token/)
})

test('target drift cannot rebuild or upload a frozen request with different code', async () => {
  const state = setup()
  state.job = { ...state.job, target: { ...target, codeRevision: 'c'.repeat(40) } }
  await state.worker.tick()
  assert.equal(state.job.phase, 'queued')
  assert.match(state.job.error, /configuration/i)
  assert.ok(!state.events.includes('build'))
})

test('reconciliation binds only a verified Production deployment and records the operator note', async () => {
  const state = setup()
  state.job = { ...state.job, phase: 'submitting' }
  await state.worker.reconcile(state.job.id, { deploymentId: 'deployment', note: 'Verified in cloud console' })
  assert.equal(state.job.phase, 'deployed')
  assert.equal(state.job.reconciliation[0].note, 'Verified in cloud console')
  await assert.rejects(state.worker.reconcile(state.job.id, { deploymentId: 'other', note: 'retry' }))
})

test('reconciliation cannot bind a deployment from another project or clear uncertainty without confirmation', async () => {
  const state = setup({ pages: { async get() { return { deploymentId: 'deployment', projectId: 'other', env: 'Production', status: 'succeeded' } } } })
  state.job = { ...state.job, phase: 'submitting' }
  await assert.rejects(state.worker.reconcile(state.job.id, { deploymentId: 'deployment', note: 'checked' }))
  await assert.rejects(state.worker.reconcile(state.job.id, { confirmedAbsent: false, note: 'checked' }))
  assert.equal(state.job.phase, 'submitting')
  await state.worker.reconcile(state.job.id, { confirmedAbsent: true, note: 'No deployment created; checked console' })
  assert.equal(state.job.phase, 'failed')
})

test('database failures at the submission boundary prevent upload', async () => {
  let uploads = 0
  const state = setup({
    store: { async active() { return { id: 'build', phase: 'building', request: {}, target } }, async update() { throw new Error('database down') } },
    pages: { async deploy() { uploads++; return {} } },
  })
  await assert.rejects(state.worker.tick())
  assert.equal(uploads, 0)
})
