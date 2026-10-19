import { expect, test } from '@playwright/test'
import { navigate, startStudio } from './studio'

test('手机刷新分页历史列表仍显示当前版本且可以继续切换', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const api = await startStudio(page)
  const { snapshot, ...current } = api.records.get('ver_14cea400-a42f-4560-9770-3de88cae9d5a')!
  void snapshot
  const { snapshot: historySnapshot, ...history } = api.records.get('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')!
  void historySnapshot
  const recent = [current, ...Array.from({ length: 19 }, (_, index) => ({ ...current, id: `ver_0f768b7b-3272-4a05-a829-e77b743f9996${index}`, name: `Recent ${index}` }))]
  await page.route('**/api/v1/versions?*', route => {
    const cursor = new URL(route.request().url()).searchParams.get('cursor')
    return route.fulfill({ json: cursor ? { items: [history] } : { items: recent, nextCursor: 'older' } })
  })
  await navigate(page, 'history')
  const content = page.locator('[data-studio-content]')
  await content.getByRole('button', { name: 'Refresh', exact: true }).click()
  await content.getByRole('button', { name: 'Load more', exact: true }).click()
  const select = page.locator('.studio-version-select')
  await select.selectOption(history.id)
  await expect(page.getByTestId('studio-version-name')).toHaveText(history.name)
  await content.getByRole('button', { name: 'Refresh', exact: true }).click()
  await expect(select.locator('option')).toHaveCount(21)
  await expect(content.locator('.studio-history-list > button')).toHaveCount(20)
  await expect(select).toHaveValue(history.id)
  await expect(page.getByTestId('studio-version-name')).toHaveText(history.name)
  await select.selectOption(current.id)
  await expect(select).toHaveValue(current.id)
  await expect(page.getByTestId('studio-version-name')).toHaveText(current.name)
  expect(api.writes).toHaveLength(0)
})

test('手机取消版本切换后恢复当前选择，原生键盘可再次选择同一历史版本', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const api = await startStudio(page)
  await navigate(page, 'homepage')
  await page.getByTestId('homepage-headline-en').fill('Keep local draft')
  const select = page.locator('.studio-version-select')
  await select.selectOption('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-keep').click()
  await expect(select).toHaveValue('ver_14cea400-a42f-4560-9770-3de88cae9d5a')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.getByTestId('homepage-headline-en')).toHaveValue('Keep local draft')
  await select.focus()
  // 方向键已提交原生选择，焦点随即进入确认弹窗。
  await page.keyboard.press('ArrowDown')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-discard').click()
  await expect(select).toHaveValue('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
  expect(api.writes).toHaveLength(0)
})

test('手机历史版本载入失败时保留当前选择并允许再次选择相同目标', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await startStudio(page)
  let fail = true
  await page.route('**/api/v1/versions/ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', route => fail
    ? route.fulfill({ status: 500, json: { code: 'unavailable', message: 'Version unavailable', requestId: 'req_577e4583-a9ae-42a1-9778-e3d044c3d485' } })
    : route.fallback())
  const select = page.locator('.studio-version-select')
  await select.selectOption('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  await expect(page.getByTestId('toast-viewport')).toContainText('req_577e4583-a9ae-42a1-9778-e3d044c3d485')
  await expect(select).toHaveValue('ver_14cea400-a42f-4560-9770-3de88cae9d5a')
  await expect(select).toBeEnabled()
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  fail = false
  await select.focus()
  await page.keyboard.press('ArrowDown')
  await expect(select).toHaveValue('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
})
