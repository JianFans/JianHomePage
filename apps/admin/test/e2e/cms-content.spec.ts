import { expect, test } from '@playwright/test'
import { navigate, startStudio } from './studio'

test('站点资料保存完整版本，未保存导航受保护且历史版本只读', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'site')
  await page.getByTestId('site-brand-zh').fill('遇健我 · 新版')
  await navigate(page, 'history')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-keep').click()
  await expect(page.getByTestId('site-brand-zh')).toHaveValue('遇健我 · 新版')
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.studio-meta')).toContainText('Saved')
  expect(api.saves.at(-1)?.site.brand['zh-CN']).toBe('遇健我 · 新版')
  await page.locator('.studio-version-select').selectOption('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
  await expect(page.getByTestId('site-brand-zh')).toBeDisabled()
})
