import { expect, test } from '@playwright/test'
import { fixtureSnapshot, navigate, startStudio } from './studio'

const source = fixtureSnapshot()
for (const scenario of [
  { kind: 'video', field: 'video-title-zh', path: '/videos/0/title/zh-CN', invalid: '', valid: '修正影像', select: false },
  { kind: 'video', field: 'video-poster', path: '/videos/0/posterAssetId', invalid: '', valid: 'asset_cover_01', select: true },
  { kind: 'video', field: 'video-duration', path: '/videos/0/durationSeconds', invalid: '0', valid: '181', select: false },
  { kind: 'moment', field: 'moment-asset', path: '/moments/0/assetId', invalid: '', valid: 'asset_cover_01', select: true },
  { kind: 'moment', field: 'moment-caption-zh', path: '/moments/0/caption/zh-CN', invalid: '', valid: '修正片段', select: false },
  { kind: 'moment', field: 'moment-target-url', path: '/moments/0/target/link/url', invalid: 'http://example.com', valid: 'https://example.com', select: false },
] as const) {
  test(`跨记录诊断定位 ${scenario.field}，修正后可保存`, async ({ page }) => {
    const api = await startStudio(page)
    await navigate(page, scenario.kind)
    if (scenario.field === 'moment-target-url') await page.getByTestId('moment-target-kind').selectOption('external')
    const field = page.getByTestId(scenario.field)
    if (scenario.select) await field.selectOption(scenario.invalid)
    else await field.fill(scenario.invalid)
    const records = scenario.kind === 'video' ? source.videos : source.moments
    await page.getByTestId(`${scenario.kind}-record-${records[1]!.id}`).click()
    await page.getByTestId('snapshot-save').click()
    await expect(page.getByTestId(`${scenario.kind}-id`)).toHaveText(records[0]!.id)
    await expect(field).toBeFocused()
    await expect(field).toHaveAttribute('aria-invalid', 'true')
    await expect(field).toHaveAttribute('data-snapshot-path', scenario.path)
    const description = await field.getAttribute('aria-describedby')
    expect(description).toBeTruthy()
    await expect(page.locator(`[id="${description}"]`)).toContainText(scenario.path)
    expect(api.saves).toHaveLength(0)
    if (scenario.select) await field.selectOption(scenario.valid)
    else await field.fill(scenario.valid)
    await page.getByTestId('snapshot-save').click()
    await expect(page.locator('.studio-meta')).toContainText('Saved')
    expect(api.saves).toHaveLength(1)
    await expect(field).not.toHaveAttribute('aria-invalid', 'true')
  })
}
