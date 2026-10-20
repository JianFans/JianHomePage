import { expect, test } from '@playwright/test'
import { backend, navigate, selectVersion } from './studio'

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
  await page.route('**/api/v1/publishes/pub_d12f724e-a67b-4212-ae95-44b9aa602de8/refresh', route => {
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
  await page.getByRole('button', { name: 'Refresh · building', exact: true }).click()
  await expect(page.locator('.studio-meta')).toContainText(selectedPrevious ? 'Archived' : 'Published')
  await expect(page.locator('.studio-meta')).toContainText(selectedPrevious ? 'Revision 3' : 'Revision 6')
  await expect(page.getByTestId('version-ver_14cea400-a42f-4560-9770-3de88cae9d5a')).toContainText('Published')
  await expect(page.getByTestId('version-ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')).toContainText('Archived')
  await expect(page.getByRole('button', { name: 'Rollback', exact: true })).toBeEnabled()
})
