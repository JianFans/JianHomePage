import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'

async function tabUntilFocused(
  page: import('@playwright/test').Page,
  locator: import('@playwright/test').Locator,
) {
  for (let index = 0; index < 40; index += 1) {
    await page.keyboard.press('Tab')
    if (await locator.evaluate(element => element === document.activeElement)) {
      return true
    }
  }
  return false
}

test.use({
  locale: 'zh-CN',
  viewport: { width: 1440, height: 1000 },
})

for (const locale of ['zh-CN', 'en-US']) {
  for (const viewport of [{ width: 1440, height: 1000 }, { width: 390, height: 844 }]) {
    test.describe(`无障碍扫描 ${locale} ${viewport.width}`, () => {
      test.use({ locale, viewport })

      test('首页通过 axe 自动无障碍扫描', async ({ page }) => {
        await page.goto('/')
        await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
        await expect(page.locator('html')).toHaveAttribute('lang', locale === 'en-US' ? 'en' : 'zh-CN')
        if (viewport.width === 390) await page.getByTestId('platform-more').first().locator('summary').click()

        const results = await new AxeBuilder({ page }).analyze()
        expect(results.violations).toEqual([])
      })
    })
  }
}

test('导航、播放、平台和语言操作都可由键盘访问', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

  const targets = [
    page.getByRole('link', { name: /遇健我.*王子健/ }),
    page.getByRole('link', { name: '音乐', exact: true }),
    page.getByRole('button', { name: 'Switch to English' }),
    page.getByTestId('preview-trigger').first(),
    page.getByRole('link', { name: 'QQ 音乐' }).first(),
  ]

  for (const target of targets) {
    expect(await tabUntilFocused(page, target)).toBe(true)
  }
})

test('所有图标操作具有可访问名称，视频出现时提供暂停控制', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toBeVisible()

  const unnamed = await page.locator('button, a, summary').evaluateAll(elements => (
    elements.flatMap((element) => {
      const style = getComputedStyle(element)
      const rect = element.getBoundingClientRect()
      if (!rect.width || !rect.height || style.display === 'none' || style.visibility === 'hidden') {
        return []
      }
      const name = element.getAttribute('aria-label')
        || element.getAttribute('title')
        || element.textContent?.trim()
      return name ? [] : [element.outerHTML]
    })
  ))
  expect(unnamed).toEqual([])

  if (await page.locator('video').count()) {
    await expect(page.getByRole('button', { name: /暂停视频|Pause video/ })).toBeVisible()
  }
})

test('减少动态效果时不加载自动播放视频', async ({ browser }) => {
  const context = await browser.newContext({
    reducedMotion: 'reduce',
    viewport: { width: 1440, height: 1000 },
  })
  const page = await context.newPage()
  await page.goto('/')

  await expect(page.locator('[data-testid="hero-media"] video[autoplay]')).toHaveCount(0)
  await context.close()
})
