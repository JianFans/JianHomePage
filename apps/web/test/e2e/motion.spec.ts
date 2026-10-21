import { expect, test } from '@playwright/test'

for (const width of [1440, 390]) test(`轮播与滚动动效尊重实时系统偏好，宽度=${width}`, async ({ page }) => {
  await page.setViewportSize({ width, height: 900 })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await page.goto('/')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('yujian:locale'))).toMatch(/^(zh-CN|en)$/)
  await page.getByRole('button', { name: /下一项|Next slide/ }).click()
  const headline = page.locator('.hero-headline')
  await expect.poll(() => headline.evaluate(element => element.getAnimations().some(animation => animation instanceof CSSAnimation && animation.animationName === 'site-slide-in'))).toBe(true)
  // 动画期间仍可立即切换，不排队，也不保留旧的媒体层。
  await page.getByRole('button', { name: /上一项|Previous slide/ }).click()
  await expect(page.getByTestId('hero-media')).toHaveCount(1)
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await page.getByRole('button', { name: /下一项|Next slide/ }).click()
  await expect.poll(() => headline.evaluate(element => element.getAnimations().length)).toBe(0)
  const heading = page.locator('#music .section-heading')
  await heading.scrollIntoViewIfNeeded()
  await expect(heading).toBeVisible()
  expect(await heading.evaluate(element => getComputedStyle(element).opacity)).toBe('1')
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  if (await page.evaluate(() => CSS.supports('animation-timeline: view()'))) {
    await expect.poll(() => heading.evaluate(element => element.getAnimations().some(animation => animation instanceof CSSAnimation && animation.animationName === 'site-reveal'))).toBe(true)
  }
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
})

test('禁用 JavaScript 时公开内容和锚点仍完整可访问', async ({ browser, baseURL }) => {
  const context = await browser.newContext({ javaScriptEnabled: false, reducedMotion: 'reduce', baseURL })
  try {
    const page = await context.newPage()
    await page.goto('/')
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    for (const id of ['music', 'video', 'event', 'moment', 'artist']) {
      const section = page.locator(`#${id}`)
      await expect(section).toBeVisible()
      expect(await section.evaluate(element => getComputedStyle(element).opacity)).toBe('1')
    }
  } finally { await context.close() }
})
