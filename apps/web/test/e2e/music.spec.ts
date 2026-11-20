import { expect, test } from '@playwright/test'

test.use({ locale: 'zh-CN', viewport: { width: 360, height: 780 } })

test('从首页进入音乐目录，查看曲目平台并使用同一个试听播放器', async ({ page }) => {
  const errors: string[] = []
  page.on('pageerror', error => errors.push(error.message))
  page.on('console', message => { if (/hydration/i.test(message.text())) errors.push(message.text()) })
  await page.goto('/')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('zh-CN')
  await page.getByRole('link', { name: '查看全部音乐' }).click()
  await expect(page).toHaveURL(/\/music\/$/)
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('全部音乐作品')
  await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/music/')
  await expect(page.locator('meta[property="og:url"]')).toHaveAttribute('content', 'https://yujian.me/music/')
  await expect(page.getByRole('heading', { level: 2 })).toHaveText([
    '示例作品 01', '示例作品 02', '示例作品 03', '示例作品 04', '示例作品 05',
  ])
  const unplayable = page.getByRole('listitem').filter({ hasText: '示例曲目 02' })
  await expect(unplayable.getByRole('button', { name: /试听/ })).toHaveCount(0)
  await expect(unplayable.getByRole('link', { name: /QQ 音乐/ })).toBeVisible()
  await page.getByRole('button', { name: '试听示例曲目 01' }).click()
  const dock = page.getByTestId('audio-dock')
  await expect(dock).toHaveCount(1)
  await expect(dock).toContainText('示例曲目 01')
  await dock.getByRole('button', { name: '下一首' }).click()
  await expect(dock).toContainText('示例曲目 03')
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(360)
  await page.getByRole('button', { name: 'Switch to English' }).click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('All music')
  await page.reload()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('All music')
  expect(errors).toEqual([])
})
