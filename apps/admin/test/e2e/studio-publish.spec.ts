import { expect, test } from '@playwright/test'
import { backend, navigate, selectVersion } from './studio'
import type { AdminPublishJob } from '../../utils/admin-api'

for (const selectedPrevious of [false, true]) test(`发布及刷新同步新旧版本修订，当前选择旧版本=${selectedPrevious}`, async ({ page }) => {
  const api = await backend(page)
  const record = api.records.get('ver_14cea400-a42f-4560-9770-3de88cae9d5a')!
  Object.assign(record, { status: 'in_review', reviewApproved: true })
  const previous = api.records.get('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')!
  previous.status = 'published'
  const job = { id: 'pub_d12f724e-a67b-4212-ae95-44b9aa602de8', versionId: record.id, status: 'building', snapshotKey: 'snapshots/test', snapshotChecksum: record.checksum }
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes', route => {
    record.status = 'publishing'
    record.revision++
    return route.fulfill({ status: 202, json: job })
  })
  await page.route('**/api/v1/publishes/pub_d12f724e-a67b-4212-ae95-44b9aa602de8', route => {
    expect(route.request().method()).toBe('GET')
    record.status = 'published'
    record.revision++
    job.status = 'succeeded'
    previous.status = 'archived'
    previous.revision++
    return route.fulfill({ json: job })
  })
  await page.goto('/')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.getByTestId('studio-preview')).toBeEnabled()
  await navigate(page, 'history')
  await page.getByRole('button', { name: 'Publish', exact: true }).click()
  await page.getByRole('button', { name: 'Confirm', exact: true }).click()
  await expect(page.locator('.studio-meta')).toContainText('Revision 5')
  await expect(page.getByTestId('version-ver_14cea400-a42f-4560-9770-3de88cae9d5a')).toContainText('Publishing')
  await expect(page.getByRole('button', { name: 'Publish', exact: true })).toHaveCount(0)
  if (selectedPrevious) {
    await selectVersion(page, previous.id)
    await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
  }
  await page.getByTestId('publish-refresh').click()
  await expect(page.locator('.studio-meta')).toContainText(selectedPrevious ? 'Archived' : 'Published')
  await expect(page.locator('.studio-meta')).toContainText(selectedPrevious ? 'Revision 3' : 'Revision 6')
  await expect(page.getByTestId('version-ver_14cea400-a42f-4560-9770-3de88cae9d5a')).toContainText('Published')
  await expect(page.getByTestId('version-ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')).toContainText('Archived')
  await expect(page.getByRole('button', { name: 'Rollback', exact: true })).toBeEnabled()
})

for (const module of ['overview', 'history'] as const) test(`${module} 面板手动刷新只读状态，不发送发布写请求`, async ({ page }) => {
  const api = await backend(page)
  const job = task()
  job.build = { phase: 'submitting', error: 'Deployment outcome unknown', configurationMismatch: true }
  api.records.get(job.versionId)!.status = 'publishing'
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes/latest', route => route.fulfill({ json: job }))
  await page.route(`**/api/v1/publishes/${job.id}`, route => {
    expect(route.request().method()).toBe('GET')
    return route.fulfill({ json: { ...job, build: { phase: 'deployed', deploymentId: 'dp-read-only', configurationMismatch: false } } })
  })
  await page.goto('/')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.getByTestId('studio-preview')).toBeEnabled()
  await expect(page.getByTestId('publish-status')).toContainText('Verification required')
  if (module === 'history') {
    await navigate(page, module)
    await expect(page.locator('[data-module="history"]')).toBeVisible()
  }
  await page.getByTestId('publish-refresh').click()
  await expect(page.getByTestId('publish-status')).toContainText('dp-read-only')
  expect(api.writes).toHaveLength(0)
})

/** 云端仅替换 HTTP 边界，任务恢复、表单和轮询由真实管理端驱动。 */
function task(): AdminPublishJob {
  return { id: 'pub_d12f724e-a67b-4212-ae95-44b9aa602de8', versionId: 'ver_14cea400-a42f-4560-9770-3de88cae9d5a', operation: 'publish', status: 'building', buildId: 'build_981ab54b-e9e9-4230-8803-a6af5c970ad3', snapshotKey: 'snapshots/test', snapshotChecksum: 'sha256:browser', build: { phase: 'deployed', deploymentId: 'dp-test', configurationMismatch: false } }
}

for (const mismatch of [false, true]) test(`重新进入恢复持久任务并自动显示部署终态，不创建新任务，配置变化=${mismatch}`, async ({ page }) => {
  const api = await backend(page)
  const job = task()
  job.build!.configurationMismatch = mismatch
  const record = api.records.get(job.versionId)!
  record.status = 'publishing'
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes/latest', route => route.fulfill({ json: job }))
  await page.route(`**/api/v1/publishes/${job.id}`, route => {
    record.status = 'published'
    record.revision++
    job.status = 'succeeded'
    job.build!.phase = 'succeeded'
    return route.fulfill({ json: job })
  })
  await page.goto('/')
  await expect(page.getByTestId('publish-status')).toContainText('Deployment accepted')
  await expect(page.getByTestId('publish-status')).toContainText('dp-test')
  await expect(page.getByTestId('publish-status')).not.toContainText('Restore its original configuration')
  await expect(page.getByTestId('publish-status').getByRole('status')).toHaveText('Published', { timeout: 12000 })
  await expect(page.locator('.studio-meta')).toContainText('Published')
  expect(api.writes).toHaveLength(0)
  await page.reload()
  await expect(page.getByTestId('publish-status').getByRole('status')).toHaveText('Published')
  expect(api.writes).toHaveLength(0)
})

for (const phase of ['succeeded', 'failed'] as const) test(`配置变化不阻止已结束构建 ${phase} 的结果同步`, async ({ page }) => {
  const api = await backend(page)
  const job = task()
  job.build = { phase, configurationMismatch: true }
  const record = api.records.get(job.versionId)!
  record.status = 'publishing'
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes/latest', route => route.fulfill({ json: job }))
  await page.route(`**/api/v1/publishes/${job.id}`, route => {
    job.status = phase
    record.status = phase === 'succeeded' ? 'published' : 'in_review'
    return route.fulfill({ json: job })
  })
  await page.goto('/')
  const panel = page.getByTestId('publish-status')
  await expect(panel.getByRole('status')).toHaveText('Synchronizing publish result')
  await expect(panel).not.toContainText('Restore its original configuration')
  await expect(panel.getByRole('status')).toHaveText(phase === 'succeeded' ? 'Published' : 'Failed', { timeout: 12000 })
  await expect(panel).not.toContainText('Restore its original configuration')
  expect(api.writes).toHaveLength(0)
})

test('失败任务确认后以新幂等键重试，保留旧版本选择', async ({ page }) => {
  const api = await backend(page)
  const job = task()
  job.status = 'failed'
  job.build = { phase: 'failed', error: 'Preparation failed', configurationMismatch: false }
  const record = api.records.get(job.versionId)!
  Object.assign(record, { status: 'in_review', reviewApproved: true })
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes/latest', route => route.fulfill({ json: job }))
  const keys: string[] = []
  await page.route('**/api/v1/publishes', route => {
    expect(route.request().postDataJSON()).toEqual({ versionId: record.id })
    keys.push(route.request().headers()['idempotency-key']!)
    return route.fulfill({ status: 202, json: { ...job, id: `pub_retry_${keys.length}` } })
  })
  await page.goto('/')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await selectVersion(page, 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  for (let i = 0; i < 2; i++) {
    await page.getByTestId('publish-retry').click()
    expect(keys).toHaveLength(i)
    await page.getByTestId('publish-confirm').click()
    await expect(page.getByTestId('publish-status')).toContainText(`pub_retry_${i + 1}`)
  }
  expect(keys[0]).toBeTruthy()
  expect(keys[1]).not.toBe(keys[0])
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
})

test('核实失败保留输入，成功后刷新失败不重新核实并自动同步', async ({ page }) => {
  const api = await backend(page)
  const job = task()
  job.build = { phase: 'submitting', error: 'Deployment outcome unknown', configurationMismatch: true }
  api.records.get(job.versionId)!.status = 'publishing'
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/api/v1/publishes/latest', route => route.fulfill({ json: job }))
  let attempts = 0
  await page.route(`**/api/v1/publisher/builds/${job.buildId}/recovery`, route => {
    expect(route.request().headers()['x-csrf-token']).toBe('csrf-browser-contract')
    expect(route.request().postDataJSON()).toEqual({ confirmedAbsent: true, note: 'Checked original project' })
    if (++attempts === 1) return route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Unavailable' } })
    job.build = { phase: 'failed', deploymentId: '', error: 'Confirmed absent', configurationMismatch: true }
    return route.fulfill({ json: job.build })
  })
  let reads = 0
  await page.route(`**/api/v1/publishes/${job.id}`, route => {
    if (++reads === 1) return route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Unavailable' } })
    job.status = 'failed'
    api.records.get(job.versionId)!.status = 'in_review'
    return route.fulfill({ json: job })
  })
  await page.goto('/')
  await expect(page.getByTestId('publish-status')).toContainText('Verification required')
  await page.getByTestId('publish-recover').click()
  await expect(page.getByTestId('publish-confirm')).toBeDisabled()
  await page.getByRole('textbox', { name: 'Verification note (one line)' }).fill('Checked original project')
  await page.getByRole('checkbox').check()
  await page.getByTestId('publish-confirm').click()
  await expect(page.getByTestId('toast-viewport')).toContainText('Unavailable')
  await expect(page.getByRole('textbox', { name: 'Verification note (one line)' })).toHaveValue('Checked original project')
  await page.getByTestId('publish-confirm').click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByTestId('publish-status')).toContainText('Synchronizing publish result')
  await expect(page.getByTestId('publish-status')).toContainText('Status could not be read')
  await expect(page.getByTestId('publish-recover')).toHaveCount(0)
  await expect(page.getByTestId('publish-retry')).toBeVisible({ timeout: 12000 })
  expect(attempts).toBe(2)
})
