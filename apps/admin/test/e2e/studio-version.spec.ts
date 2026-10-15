import { expect, test } from '@playwright/test'
import { navigate, startStudio } from './studio'

test('手机取消版本切换后恢复当前选择，原生键盘可再次选择同一历史版本', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const api = await startStudio(page)
  await navigate(page, 'homepage')
  await page.getByTestId('homepage-headline-en').fill('Keep local draft')
  const select = page.locator('.studio-version-select')
  await select.selectOption('ver_history')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-keep').click()
  await expect(select).toHaveValue('ver_current')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.getByTestId('homepage-headline-en')).toHaveValue('Keep local draft')
  await select.focus()
  // 方向键已提交原生选择，焦点随即进入确认弹窗。
  await page.keyboard.press('ArrowDown')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-discard').click()
  await expect(select).toHaveValue('ver_history')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
  expect(api.writes).toHaveLength(0)
})

test('手机历史版本载入失败时保留当前选择并允许再次选择相同目标', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await startStudio(page)
  let fail = true
  await page.route('**/api/v1/versions/ver_history', route => fail
    ? route.fulfill({ status: 500, json: { code: 'unavailable', message: 'Version unavailable', requestId: 'req_version' } })
    : route.fallback())
  const select = page.locator('.studio-version-select')
  await select.selectOption('ver_history')
  await expect(page.getByTestId('toast-viewport')).toContainText('req_version')
  await expect(select).toHaveValue('ver_current')
  await expect(select).toBeEnabled()
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  fail = false
  await select.focus()
  await page.keyboard.press('ArrowDown')
  await expect(select).toHaveValue('ver_history')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
})
