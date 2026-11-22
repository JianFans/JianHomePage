import { expect, test } from '@playwright/test'

test.use({ locale: 'zh-CN' })

test('静态快照首次加载失败后可重试并只水合一次', async ({ page }) => {
  test.skip(!process.env.YUJIAN_EMBEDDED_SITE, 'Only applies to the Go-generated distribution')
  let attempts = 0
  await page.context().route('**/site/snapshot-*.json', async (route) => {
    attempts++
    if (attempts === 1) {
      await route.fulfill({ status: 503, body: 'temporarily unavailable' })
      return
    }
    await route.continue()
  })

  await page.goto('/')
  const alert = page.getByRole('alert')
  await expect(alert).toContainText('页面内容加载失败')
  await alert.getByRole('button', { name: '重试' }).click()
  await expect(page.getByRole('button', { name: 'Switch to English' })).toBeVisible()
  await expect(page.getByTestId('audio-dock-host')).toHaveCount(1)
  await expect(alert).toBeHidden()
  expect(attempts).toBe(2)
})

test('Go 生成页面正确 hydration，保留语言偏好且没有运行时内容 API', async ({ page }) => {
  test.skip(!process.env.YUJIAN_EMBEDDED_SITE, 'Only applies to the Go-generated distribution')
  const failures: string[] = []
  const requests: string[] = []
  const snapshotSources: string[] = []
  page.on('pageerror', error => failures.push(error.message))
  page.on('console', message => { if (message.type() === 'error' || message.text().includes('Hydration')) failures.push(message.text()) })
  page.on('request', request => { if (request.resourceType() === 'fetch' || request.resourceType() === 'xhr') requests.push(request.url()) })
  await page.goto('/')
  await expect(page.locator('#yujian-snapshot')).toBeAttached()
  snapshotSources.push((await page.locator('#yujian-snapshot').getAttribute('data-src')) || '')
  await expect(page.getByRole('button', { name: 'Switch to English' })).toBeVisible()
  await page.goto('/video/')
  await expect(page.locator('#yujian-snapshot')).toHaveAttribute('data-page', 'video')
  snapshotSources.push((await page.locator('#yujian-snapshot').getAttribute('data-src')) || '')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('全部影像')
  await expect(page.getByRole('heading', { level: 2 })).toHaveCount(3)
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/video/')
  await page.getByRole('button', { name: 'Switch to English' }).click()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('All videos')
  await page.goto('/events/')
  await expect(page.locator('#yujian-snapshot')).toHaveAttribute('data-page', 'events')
  snapshotSources.push((await page.locator('#yujian-snapshot').getAttribute('data-src')) || '')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Live events')
  await expect(page.getByRole('heading', { level: 2 })).toHaveCount(2)
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/events/')
  await page.goto('/moments/')
  await expect(page.locator('#yujian-snapshot')).toHaveAttribute('data-page', 'moments')
  snapshotSources.push((await page.locator('#yujian-snapshot').getAttribute('data-src')) || '')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Moments')
  await expect(page.locator('[data-testid="moment-card"]')).toHaveCount(3)
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/moments/')
  await page.goto('/')
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('lang', 'en')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('en')
  expect(new Set(snapshotSources)).toEqual(new Set([snapshotSources[0]]))
  expect(snapshotSources[0]).toMatch(/^\/site\/snapshot-[a-f0-9]{64}\.json$/)
  expect(requests.filter(url => url.includes('/api/'))).toEqual([])
  expect(requests.filter(url => new URL(url).pathname === snapshotSources[0]).length).toBeGreaterThan(0)
  expect(failures).toEqual([])
})
