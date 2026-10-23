import { expect, test } from '@playwright/test'
import { fixtureSnapshot, navigate, startStudio, openTools } from './studio'

test('站点资料保存完整版本，未保存导航受保护且历史版本只读', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'site')
  await page.getByTestId('site-brand-zh').fill('遇健我 · 新版')
  await navigate(page, 'history')
  await expect(page.getByTestId('leave-dialog')).toBeVisible()
  await page.getByTestId('leave-keep').click()
  await expect(page.getByTestId('site-brand-zh')).toHaveValue('遇健我 · 新版')
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.studio-meta')).toContainText('Saved')
  expect(api.saves.at(-1)?.site.brand['zh-CN']).toBe('遇健我 · 新版')
  if (await page.locator('.studio-version-select').isVisible()) await page.locator('.studio-version-select').selectOption('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  else await page.getByTestId('version-ver_8bb3a437-061d-4dcd-805e-7d5e6f716921').click()
  await expect(page.getByTestId('studio-version-name')).toHaveText('Previous version')
  await expect(page.getByTestId('site-brand-zh')).toBeDisabled()
  await expect(page.getByTestId('site-og-image').locator('..').getByTestId('asset-picker-open')).toBeDisabled()
})

test('删除确认列出影响，只修改当前快照并通过顶部保存', async ({ page }) => {
  const value = fixtureSnapshot()
  const id = 'video_609bc694-1202-4501-9e77-a5c35fb663eb'
  value.videos.push({ ...value.videos[0]!, id, title: { 'zh-CN': '待删除影像', en: 'Remove me' } })
  const api = await startStudio(page, value)
  await navigate(page, 'video')
  await page.getByTestId('content-delete-select').selectOption(id)
  await page.getByTestId('content-delete-open').click()
  await expect(page.getByRole('dialog')).toContainText('Remove me')
  await page.getByTestId('content-delete-ack').check()
  await page.getByTestId('content-delete-confirm').click()
  await expect(page.getByTestId('content-delete-select').locator(`option[value="${id}"]`)).toHaveCount(0)
  expect(api.saves).toHaveLength(0)
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.studio-meta')).toContainText('Saved')
  expect(api.saves.at(-1)?.videos.some(video => video.id === id)).toBe(false)
})

test('删除存在跨内容引用的作品时列出阻止路径且不修改快照', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'music')
  await page.getByTestId('content-delete-open').click()
  const dialog = page.getByRole('dialog')
  await expect(dialog).toContainText('/heroSlides/2/releaseId')
  await expect(dialog).toContainText('/heroSlides/2/target/contentId')
  await page.getByTestId('content-delete-ack').check()
  await expect(page.getByTestId('content-delete-confirm')).toBeDisabled()
  await page.getByTestId('dialog-close').click()
  expect(api.writes).toHaveLength(0)
  await expect(page.locator('.studio-meta')).toContainText('Saved')
})

test('字段中选择素材库图片，保留未保存影像标题并一起保存', async ({ page }) => {
  const api = await startStudio(page)
  const id = 'asset_609bc694-1202-4501-9e77-a5c35fb663eb'
  await page.route(/\/api\/v1\/assets(?:\?.*)?$/, route => route.fulfill({ json: { items: [{
    id, status: 'ready', src: 'https://media.example/new.webp', metadata: { fileName: 'new.webp', contentType: 'image/webp', declaredSize: 7, checksum: 'sha256:' + 'a'.repeat(64), width: 64, height: 64 }, rights: { source: { 'zh-CN': '授权' } },
  }] } }))
  await navigate(page, 'video')
  await page.getByTestId('video-title-zh').fill('保留尚未保存的影像标题')
  const trigger = page.getByTestId('video-poster').locator('..').getByTestId('asset-picker-open')
  await trigger.click()
  await expect(page.getByTestId('dialog-close')).toBeFocused()
  await page.getByTestId('picker-library').click()
  await page.getByTestId('asset-alt-zh').fill('新封面')
  await page.getByTestId('asset-insert').click()
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(trigger).toBeFocused()
  await expect(page.getByTestId('video-title-zh')).toHaveValue('保留尚未保存的影像标题')
  await expect(page.getByTestId('video-poster')).toHaveValue(id)
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.studio-meta')).toContainText('Saved')
  expect(api.saves.at(-1)?.videos[0]?.title['zh-CN']).toBe('保留尚未保存的影像标题')
  expect(api.saves.at(-1)?.videos[0]?.posterAssetId).toBe(id)
  expect(api.saves.at(-1)?.assets.some(asset => asset.id === id)).toBe(true)
})

test('各模块素材字段保持可访问名称，键盘关闭选择器后恢复触发焦点', async ({ page }) => {
  await startStudio(page)
  const fields = {
    site: ['site-og-image', 'site-portrait'],
    homepage: ['homepage-hero-asset', 'homepage-mobile-asset', 'homepage-poster-asset'],
    music: ['music-cover', 'track-preview'],
    video: ['video-poster', 'video-asset'],
    event: ['event-poster'],
    moment: ['moment-asset'],
  } as const
  for (const module of Object.keys(fields) as (keyof typeof fields)[]) {
    await navigate(page, module)
    if (module === 'music') {
      await page.getByTestId(`music-release-${fixtureSnapshot().releases[0]!.id}`).click()
    }
    for (const field of fields[module]) await expect(page.getByTestId(field).first()).toHaveAccessibleName(/\S/)
  }
  const trigger = page.getByTestId('moment-asset').locator('..').getByTestId('asset-picker-open')
  await trigger.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('dialog-close')).toBeFocused()
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toHaveCount(0)
  await expect(trigger).toBeFocused()
})

test('历史对比显示字段变化且保留当前版本和未保存 JSON', async ({ page }) => {
  const api = await startStudio(page)
  api.records.get('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')!.snapshot.site.brand['zh-CN'] = '历史站名'
  await openTools(page)
  const editor = page.getByRole('textbox', { name: 'JSON snapshot editor' })
  const original = await editor.inputValue()
  await editor.fill(original + '\n')
  await page.getByTestId('dialog-close').click()
  await navigate(page, 'history')
  await expect(page.getByTestId('version-comparison')).toContainText('/site/brand/zh-CN')
  await expect(page.getByTestId('version-comparison')).toContainText('历史站名')
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.locator('.studio-meta')).toContainText('Unsaved')
  await openTools(page)
  await expect(page.getByRole('textbox', { name: 'JSON snapshot editor' })).toHaveValue(original + '\n')
  expect(api.writes).toHaveLength(0)
})
