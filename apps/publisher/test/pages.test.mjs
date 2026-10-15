import assert from 'node:assert/strict'
import { tmpdir } from 'node:os'
import test from 'node:test'
import { createPagesProvider } from '../src/pages.mjs'

/** SDK-shaped deployment; provider error details must never enter public state. */
function deployment(overrides = {}) {
  return { deploymentId: 'deployment-1', projectId: 'project-1', env: 'Production', status: 'Success', previewUrl: 'https://preview.example.test/', code: 'private-provider-detail', ...overrides }
}

test('deploys once to Production and returns only the stable safe deployment identity', async () => {
  const original = deployment({ previewUrl: 'https://user:private@example.test/?token=private', error: 'provider-private-detail' })
  let count = 0
  const provider = createPagesProvider({ deployments: { deploy: async (input) => {
    count++
    assert.deepEqual(input, { projectId: 'project-1', artifact: { directory: tmpdir() }, env: 'Production', wait: false })
    return original
  } } })
  assert.deepEqual(await provider.deploy(tmpdir(), 'project-1'), { deploymentId: 'deployment-1', projectId: 'project-1', env: 'Production' })
  assert.equal(count, 1)
})

for (const [sdkStatus, status] of Object.entries({ Pending: 'building', Process: 'building', Success: 'succeeded', Failed: 'failed', Timeout: 'failed', Cancelled: 'failed', Invalid: 'failed' })) {
  test(`maps SDK ${sdkStatus} to ${status} and removes raw provider fields`, async () => {
    const provider = createPagesProvider({ deployments: { get: async (input) => {
      assert.deepEqual(input, { projectId: 'project-1', deploymentId: 'deployment-1' })
      return deployment({ status: sdkStatus })
    } } })
    const result = await provider.get('project-1', 'deployment-1')
    assert.equal(result.status, status)
    assert.equal(result.previewUrl, 'https://preview.example.test/')
    assert.equal(result.code, undefined)
    assert.equal(JSON.stringify(result).includes('private-provider-detail'), false)
    assert.equal(result.error, status === 'failed' ? 'Pages deployment failed' : undefined)
  })
}

for (const overrides of [{ deploymentId: '' }, { projectId: 'other' }, { env: 'Preview' }]) {
  test(`rejects mismatched SDK identity ${JSON.stringify(overrides)}`, async () => {
    const provider = createPagesProvider({ deployments: {
      deploy: async () => deployment(overrides), get: async () => deployment(overrides),
    } })
    await assert.rejects(provider.deploy(tmpdir(), 'project-1'), /Invalid Pages deployment/)
    await assert.rejects(provider.get('project-1', 'deployment-1'), /Invalid Pages deployment/)
  })
}

test('rejects a different deployment ID when querying a known deployment', async () => {
  const provider = createPagesProvider({ deployments: { get: async () => deployment({ deploymentId: 'other' }) } })
  await assert.rejects(provider.get('project-1', 'deployment-1'), /Invalid Pages deployment/)
})

for (const status of ['future-status', undefined]) {
  test(`throws on ${status ?? 'missing'} status so the worker can retain building state`, async () => {
    const provider = createPagesProvider({ deployments: { get: async () => deployment({ status }) } })
    await assert.rejects(provider.get('project-1', 'deployment-1'), /Unknown Pages deployment status/)
  })
}

for (const previewUrl of ['http://example.test/', 'https://user:password@example.test/', 'https://example.test/?token=secret', 'https://example.test/#secret', '/relative', 'javascript:alert(1)']) {
  test(`discards unsafe preview URL ${previewUrl}`, async () => {
    const provider = createPagesProvider({ deployments: { get: async () => deployment({ previewUrl }) } })
    assert.equal((await provider.get('project-1', 'deployment-1')).previewUrl, undefined)
  })
}

test('does not retry rejected SDK writes or propagate provider error contents', async () => {
  let calls = 0
  const provider = createPagesProvider({ deployments: { deploy: async () => { calls++; throw new Error('private-provider-token') } } })
  await assert.rejects(provider.deploy(tmpdir(), 'project-1'), error => error.message === 'Pages deployment request failed')
  assert.equal(calls, 1)
})
