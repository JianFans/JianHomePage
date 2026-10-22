import { expect, test } from '@playwright/test'
import { backend, navigate, startStudio } from './studio'

test('慢保存尺寸稳定，只显示实际操作的加载态，并实时响应减少动态效果', async ({ page }) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  const api = await startStudio(page)
  await navigate(page, 'video')
  await page.getByTestId('video-title-en').fill('Loading feedback')
  const save = page.getByTestId('snapshot-save')
  const before = (await save.boundingBox())!
  api.hold = true
  try {
    await save.click()
    await expect(save).toHaveAttribute('aria-busy', 'true')
    await expect(save).toHaveAccessibleName('Save draft')
    await expect(page.getByTestId('version-clone')).not.toHaveAttribute('aria-busy', 'true')
    const pending = (await save.boundingBox())!
    expect(Math.abs(pending.width - before.width)).toBeLessThanOrEqual(1)
    expect(Math.abs(pending.height - before.height)).toBeLessThanOrEqual(1)
    const indicator = save.locator('.activity-indicator')
    await expect.poll(() => indicator.evaluate(element => element.getAnimations().length)).toBeGreaterThan(0)
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect.poll(() => indicator.evaluate(element => element.getAnimations().length)).toBe(0)
    await save.dispatchEvent('click')
    expect(api.saves).toHaveLength(1)
    api.finish!()
    await expect(save).not.toHaveAttribute('aria-busy', 'true')
    await expect(save).toBeEnabled()
    await expect(page.locator('.studio-meta')).toContainText('Saved')
  } finally { api.finish?.() }
})

test('初始化骨架及预览等待只存在于真实加载期间，握手前不可聚焦 iframe', async ({ page }, testInfo) => {
  await page.emulateMedia({ reducedMotion: 'no-preference' })
  await backend(page)
  let releaseSession!: () => void
  const sessionWait = new Promise<void>((resolve) => { releaseSession = resolve })
  await page.route('**/api/v1/auth/session', async (route) => { await sessionWait; await route.fallback() })
  await page.goto('/')
  try {
    await expect(page.getByTestId('studio-loading')).toHaveAttribute('aria-busy', 'true')
    await expect(page.locator('.loading-skeleton').filter({ visible: true }).first()).toBeVisible()
    await page.screenshot({ path: testInfo.outputPath('workspace-loading.png') })
  } finally { releaseSession() }
  await expect(page.getByTestId('studio-version-name')).toBeVisible()
  let releasePreview!: () => void
  const previewWait = new Promise<void>((resolve) => { releasePreview = resolve })
  await page.route('**/preview', async (route) => { await previewWait; await route.continue() })
  await page.getByTestId('studio-preview').click()
  const stage = page.locator('.homepage-preview__stage')
  try {
    await expect(stage).toHaveAttribute('aria-busy', 'true')
    await expect(page.getByTestId('preview-waiting')).toBeVisible()
    await expect(page.getByTestId('preview-frame')).toHaveAttribute('inert', '')
    await page.screenshot({ path: testInfo.outputPath('preview-loading.png') })
  } finally { releasePreview() }
  await expect(stage).toHaveAttribute('aria-busy', 'false')
  await expect(page.getByTestId('preview-frame')).not.toHaveAttribute('inert', '')
  await expect(page.getByTestId('preview-waiting')).toHaveCount(0)
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('studio-preview')).toBeFocused()
})

test('素材首次读取失败退出骨架并恢复刷新入口', async ({ page }) => {
  await startStudio(page)
  let release!: () => void
  const wait = new Promise<void>((resolve) => { release = resolve })
  await page.route(/\/api\/v1\/assets(?:\?.*)?$/, async (route) => { await wait; await route.fulfill({ status: 503, json: { code: 'unavailable', message: 'Unavailable' } }) })
  await navigate(page, 'assets')
  try {
    await expect(page.getByTestId('asset-skeleton')).toBeVisible()
    await expect(page.getByTestId('asset-refresh')).toHaveAttribute('aria-busy', 'true')
  } finally { release() }
  await expect(page.getByTestId('asset-skeleton')).toHaveCount(0)
  await expect(page.getByTestId('asset-refresh')).toBeEnabled()
  await expect(page.getByTestId('asset-refresh')).not.toHaveAttribute('aria-busy', 'true')
})
