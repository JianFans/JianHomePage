import { expect, test, type Page } from '@playwright/test'
import { csrfToken, navigate, startStudio } from './studio'

/** 使用可见导航发起退出，手机先打开账号所在抽屉。 */
async function requestLogout(page: Page) {
  if (await page.locator('.studio-menu').isVisible()) await page.locator('.studio-menu').click()
  await page.getByTestId('studio-logout').filter({ visible: true }).click()
}

test('干净首页退出等待期间锁定字段和上下文，成功后清理基线', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'homepage')
  let finish!: () => void
  const response = new Promise<void>((resolve) => { finish = resolve })
  await page.route('**/api/v1/auth/logout', async route => {
    expect(route.request().headers()['x-csrf-token']).toBe(csrfToken)
    await response
    await route.fulfill({ status: 204 })
  })
  try {
    await requestLogout(page)
    await expect(page.getByTestId('homepage-headline-en')).toBeDisabled()
    await expect(page.getByTestId('homepage-hero-add')).toBeDisabled()
    await expect(page.getByTestId('snapshot-save')).toBeDisabled()
    await expect(page.getByTestId('version-clone')).toBeDisabled()
    await expect(page.getByTestId('studio-shell')).toHaveAttribute('inert', '')
    await expect(page.getByTestId('leave-dialog')).toHaveCount(0)
    const selector = page.locator('.studio-version-select')
    if (await selector.isVisible()) await expect(selector).toBeDisabled()
    else await expect(page.getByTestId('version-ver_history')).toBeDisabled()
    finish()
    await expect(page.getByTestId('studio-login')).toBeVisible()
    expect(await page.evaluate(() => !window.dispatchEvent(new Event('beforeunload', { cancelable: true })))).toBe(false)
    expect(api.writes).toHaveLength(0)
  } finally { finish() }
})

for (const module of ['homepage', 'assets'] as const) {
  test(`${module} 退出失败保留已确认放弃的输入并恢复编辑`, async ({ page }) => {
    await startStudio(page)
    await navigate(page, module)
    const input = page.getByTestId(module === 'homepage' ? 'homepage-headline-en' : 'asset-source-zh')
    await input.fill('Retain on logout failure')
    if (module === 'assets') {
      await page.getByTestId('asset-file-input').setInputFiles({ name: 'retained.webp', mimeType: 'image/webp', buffer: Buffer.from('retained-file') })
      await page.getByTestId('asset-alt-upload-zh').fill('Retained alt')
    }
    let finish!: () => void
    const response = new Promise<void>((resolve) => { finish = resolve })
    await page.route('**/api/v1/auth/logout', async route => {
      await response
      await route.fulfill({ status: 500, json: { code: 'unavailable', message: 'Logout failed', requestId: 'req_logout' } })
    })
    try {
      await requestLogout(page)
      await expect(page.getByTestId('leave-dialog')).toBeVisible()
      await page.getByTestId('leave-discard').click()
      await expect(input).toBeDisabled()
      await expect(page.getByTestId('studio-shell')).toHaveAttribute('inert', '')
      if (module === 'assets') {
        await expect(page.getByTestId('asset-file-input')).toBeDisabled()
        await expect(page.getByTestId('asset-upload')).toBeDisabled()
      }
      finish()
      await expect(page.getByTestId('toast-viewport')).toContainText('Sign-out failed')
      await expect(input).toBeEnabled()
      await expect(input).toHaveValue('Retain on logout failure')
      await input.fill('Can edit again')
      if (module === 'assets') {
        await expect(page.getByTestId('asset-upload')).toBeEnabled()
        await expect(page.getByRole('button', { name: /retained.webp/ })).toBeVisible()
      }
    } finally { finish() }
  })
}
