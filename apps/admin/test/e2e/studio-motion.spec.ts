import { expect, test } from '@playwright/test'
import { navigate, startStudio } from './studio'

test('后台动效保留导航和弹窗键盘操作，并响应减少动态效果', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await startStudio(page)
  const quickLink = page.locator('.studio-quick-links button').first()
  await expect.poll(() => quickLink.evaluate(element => element.getAnimations().some(animation => animation instanceof CSSAnimation && animation.animationName === 'studio-item-in'))).toBe(true)
  await navigate(page, 'music')
  await expect(page.locator('[data-module="music"]')).toBeVisible()
  await page.getByTestId('studio-preview').click()
  await expect(page.getByRole('dialog')).toHaveCount(1)
  await expect(page.locator('.studio-dialog')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(page.getByTestId('studio-preview')).toBeFocused()
  await page.emulateMedia({ reducedMotion: 'reduce' })
  await navigate(page, 'overview')
  await expect(quickLink).toBeVisible()
  await expect.poll(() => quickLink.evaluate(element => element.getAnimations().length)).toBe(0)
  expect(await quickLink.evaluate(element => getComputedStyle(element).transform)).toBe('none')
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
})
