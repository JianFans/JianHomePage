import { expect, test } from '@playwright/test'

test.use({ locale: 'zh-CN' })

test('Go 生成页面正确 hydration，保留语言偏好且没有运行时内容 API', async ({ page }) => {
  test.skip(!process.env.YUJIAN_EMBEDDED_SITE, 'Only applies to the Go-generated distribution')
  const failures: string[] = []
  const requests: string[] = []
  page.on('pageerror', error => failures.push(error.message))
  page.on('console', message => { if (message.type() === 'error' || message.text().includes('Hydration')) failures.push(message.text()) })
  page.on('request', request => { if (request.resourceType() === 'fetch' || request.resourceType() === 'xhr') requests.push(request.url()) })
  await page.goto('/')
  await expect(page.locator('#yujian-snapshot')).toBeAttached()
  await expect(page.getByRole('button', { name: 'Switch to English' })).toBeVisible()
  await page.getByRole('button', { name: 'Switch to English' }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('en')
  expect(requests).toEqual([])
  expect(failures).toEqual([])
})
