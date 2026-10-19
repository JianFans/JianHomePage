import { expect, test } from '@playwright/test'
import { backend, csrfToken } from './studio'

test('桌面及手机侧栏显示OIDC名称和头像，头像失败恢复占位', async ({ page }) => {
  await backend(page)
  await page.route('**/api/v1/auth/session', route => route.fulfill({ json: {
    authenticated: true, loginAvailable: true, csrfToken,
    expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
    principal: { subject: 'opaque-id-123', name: '小健管理员', picture: 'https://identity.example.test/avatar.png', roles: ['admin'] },
  } }))
  await page.route('https://identity.example.test/avatar.png', route => route.fulfill({
    contentType: 'image/png', body: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=', 'base64'),
  }))
  await page.goto('/')
  await expect(page.getByTestId('studio-shell')).toBeVisible()
  if (await page.locator('.studio-menu').isVisible()) await page.locator('.studio-menu').click()
  await expect(page.getByTestId('studio-account-name').filter({ visible: true })).toHaveText('小健管理员')
  const image = page.getByTestId('studio-avatar').filter({ visible: true })
  await expect(image).toBeVisible()
  await expect(image).toHaveAttribute('referrerpolicy', 'no-referrer')
  await expect.poll(() => image.evaluate(node => (node as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)
  await image.dispatchEvent('error')
  await expect(page.getByTestId('studio-avatar-fallback').filter({ visible: true })).toBeVisible()
})
