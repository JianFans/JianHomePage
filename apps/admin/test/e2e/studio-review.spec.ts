import { expect, test } from '@playwright/test'
import { backend, navigate } from './studio'

for (const locale of ['zh-CN', 'en'] as const) test(`审核通过后显示完成状态并锁定审核决定 ${locale}`, async ({ page }) => {
  const api = await backend(page)
  const record = api.records.get('ver_14cea400-a42f-4560-9770-3de88cae9d5a')!
  Object.assign(record, { status: 'in_review', reviewApproved: false })
  const labels = locale === 'en'
    ? { pending: 'In review', approved: 'Approved', approve: 'Approve', reject: 'Return to draft', reason: 'Return reason', publish: 'Publish' }
    : { pending: '审核中', approved: '审核通过', approve: '通过审核', reject: '退回草稿', reason: '退回原因', publish: '发布' }
  await page.addInitScript(value => localStorage.setItem('yujian:admin-locale', value), locale)
  let approvals = 0
  await page.route('**/api/v1/versions/ver_14cea400-a42f-4560-9770-3de88cae9d5a/approve', async (route) => {
    expect(route.request().postDataJSON()).toEqual({ revision: 4 })
    approvals++
    Object.assign(record, { reviewApproved: true, revision: 5 })
    await route.fulfill({ json: record })
  })
  await page.goto('/')
  await expect(page.locator('.studio-meta')).toContainText(labels.pending)
  await navigate(page, 'history')
  const review = page.getByRole('button', { name: `${labels.approve} / ${labels.reject}`, exact: true })
  await review.click()
  const dialog = page.getByRole('dialog')
  await dialog.getByRole('textbox', { name: labels.reason }).fill('Not used after approval')
  await expect(dialog.getByRole('button', { name: labels.reject, exact: true })).toBeEnabled()
  await dialog.getByRole('button', { name: labels.approve, exact: true }).click()
  await expect(dialog.getByRole('button', { name: labels.approve, exact: true })).toBeDisabled()
  await expect(dialog.getByRole('button', { name: labels.reject, exact: true })).toBeDisabled()
  await expect(dialog.getByRole('textbox', { name: labels.reason })).toBeDisabled()
  await page.getByTestId('dialog-close').click()
  await expect(page.locator('.studio-meta')).toContainText(labels.approved)
  await expect(page.locator('.studio-meta')).toContainText('5')
  await expect(page.locator('.studio-history-list')).toContainText(labels.approved)
  await expect(review).toHaveCount(0)
  await expect(page.getByRole('button', { name: labels.publish, exact: true })).toBeEnabled()
  await page.reload()
  await expect(page.locator('.studio-meta')).toContainText(labels.approved)
  await expect(page.getByTestId('version-ver_14cea400-a42f-4560-9770-3de88cae9d5a')).toContainText(labels.approved)
  await expect(page.locator('.studio-version-select')).toContainText(labels.approved)
  await expect(page.locator('.studio-history-list')).toContainText(labels.approved)
  expect(approvals).toBe(1)
  expect(api.writes.some(request => request.url().endsWith('/reject'))).toBe(false)
})
