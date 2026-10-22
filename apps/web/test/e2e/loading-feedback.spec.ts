import { expect, test } from '@playwright/test'

for (const width of [1440, 390]) test(`图片慢加载保留尺寸与内容，成功后结束等待且减少动态效果即时生效 ${width}`, async ({ page }) => {
  await page.setViewportSize({ width, height: 900 })
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  let release!: () => void
  const wait = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/media/hero-studio.webp', async (route) => { await wait; await route.continue() })
  await page.goto('/', { waitUntil: 'domcontentloaded' })
  const image = page.locator('[data-testid="hero-media"] img')
  try {
    await expect(image).toHaveAttribute('data-media-state', 'loading')
    await expect(image).toBeVisible()
    const before = (await image.boundingBox())!
    await expect.poll(() => image.evaluate(element => element.getAnimations().length)).toBeGreaterThan(0)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect.poll(() => image.evaluate(element => element.getAnimations().length)).toBe(0)
    release()
    await expect(image).toHaveAttribute('data-media-state', 'ready')
    const after = (await image.boundingBox())!
    expect(Math.abs(after.width - before.width)).toBeLessThanOrEqual(1)
    expect(Math.abs(after.height - before.height)).toBeLessThanOrEqual(1)
    expect(await image.evaluate(element => (element as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)
  } finally { release() }
})

test('音频等待反馈可取消，减少动态效果停止旋转且关闭后不会恢复播放器', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('yujian:locale', 'en'))
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  let release!: () => void
  const wait = new Promise<void>((resolve) => { release = resolve })
  await page.route('**/media/preview-sample.wav', async (route) => { await wait; await route.continue() })
  await page.goto('/')
  await expect.poll(() => page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('en')
  const play = page.getByTestId('preview-trigger').first()
  await expect(play).toHaveAccessibleName('Preview Sample Track 01')
  try {
    await play.click()
    const dock = page.getByTestId('audio-dock')
    await expect(play).toHaveAttribute('aria-busy', 'true')
    await expect(dock).toHaveAttribute('aria-busy', 'true')
    const indicator = dock.locator('.activity-indicator')
    await expect.poll(() => indicator.evaluate(element => element.getAnimations().length)).toBeGreaterThan(0)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect.poll(() => indicator.evaluate(element => element.getAnimations().length)).toBe(0)
    await page.getByRole('button', { name: 'Close player', exact: true }).click()
    release()
    await expect(dock).toHaveCount(0)
    await expect(play).not.toHaveAttribute('aria-busy', 'true')
  } finally { release() }
})
