import { expect, test } from '@playwright/test'
import { fixtureSnapshot, navigate, startStudio } from './studio'

const source = fixtureSnapshot()

for (const scenario of [
  { kind: 'music', selector: '[data-testid="music-fields"] > details [data-testid="link-url"]', path: '/releases/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
  { kind: 'music', selector: '[data-testid="track-editor"] [data-testid="link-url"]', path: '/tracks/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
  { kind: 'video', selector: '[data-testid="link-url"]', path: '/videos/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
  { kind: 'music', selector: '[data-testid="music-date"]', path: '/releases/0/releaseDate', invalid: '', valid: '2026-10-15' },
] as const) {
  test(`保存定位 ${scenario.path}，展开折叠区域并在修正后保存`, async ({ page }) => {
    const api = await startStudio(page)
    await navigate(page, scenario.kind)
    if (scenario.kind === 'music') await page.getByTestId(`music-release-${source.releases[0]!.id}`).click()
    const field = page.locator(scenario.selector).first()
    await expect(field).toBeAttached()
    const details = field.locator('xpath=ancestor::details[1]')
    if (await details.count()) await details.locator('summary').first().click()
    await field.fill(scenario.invalid)
    expect(await field.evaluate((element: HTMLInputElement) => element.validity.valid)).toBe(true)
    if (await details.count()) await details.locator('summary').first().click()
    if (scenario.kind === 'video') await page.getByTestId(`video-record-${source.videos[1]!.id}`).click()
    await page.getByTestId('snapshot-save').click()
    await expect(field).toBeFocused()
    await expect(field).toHaveAttribute('aria-invalid', 'true')
    await expect(field).toHaveAttribute('data-snapshot-path', scenario.path)
    if (await details.count()) await expect(details).toHaveAttribute('open', '')
    const description = await field.getAttribute('aria-describedby')
    expect(description).toBeTruthy()
    await expect(page.locator(`[id="${description}"]`)).toContainText(scenario.path)
    expect(api.saves).toHaveLength(0)
    await field.fill(scenario.valid)
    await page.getByTestId('snapshot-save').click()
    await expect(page.locator('.studio-meta')).toContainText('Saved')
    expect(api.saves).toHaveLength(1)
    await expect(field).not.toHaveAttribute('aria-invalid', 'true')
  })
}

for (const operation of ['add', 'remove'] as const) {
  test(`宣传位 ${operation} 后跨记录定位错误，修正后可保存`, async ({ page }) => {
    const api = await startStudio(page)
    await navigate(page, 'homepage')
    await page.getByTestId(`homepage-hero-${operation}`).click()
    const invalidId = await page.getByTestId('homepage-hero-id').innerText()
    if (operation === 'add') await page.getByTestId('homepage-headline-zh').fill('新宣传位')
    await page.getByTestId('homepage-hero-asset').selectOption('')
    const otherId = operation === 'add' ? source.heroSlides[0]!.id : source.heroSlides[2]!.id
    await page.getByTestId(`homepage-hero-${otherId}`).click()
    await page.getByTestId('snapshot-save').click()
    await expect(page.getByTestId('homepage-hero-id')).toHaveText(invalidId)
    const asset = page.getByTestId('homepage-hero-asset')
    const path = `/heroSlides/${operation === 'add' ? source.heroSlides.length : 0}/assetId`
    await expect(asset).toBeFocused()
    await expect(asset).toHaveAttribute('aria-invalid', 'true')
    await expect(asset).toHaveAttribute('data-snapshot-path', path)
    expect(api.saves).toHaveLength(0)
    await asset.selectOption('asset_2f183eda-5f01-4c34-93f6-47739c136bb2')
    await page.getByTestId('snapshot-save').click()
    await expect(page.locator('.studio-meta')).toContainText('Saved')
    expect(api.saves).toHaveLength(1)
    expect(api.saves[0]!.heroSlides.find(hero => hero.id === invalidId)?.assetId).toBe('asset_2f183eda-5f01-4c34-93f6-47739c136bb2')
    await expect(asset).not.toHaveAttribute('aria-invalid', 'true')
  })
}

for (const scenario of [
  { kind: 'event', field: 'event-detail-url', path: '/events/0/detailUrl', invalid: 'http://example.com', valid: 'https://example.com', select: false },
  { kind: 'video', field: 'video-title-zh', path: '/videos/0/title/zh-CN', invalid: '', valid: '修正影像', select: false },
  { kind: 'video', field: 'video-poster', path: '/videos/0/posterAssetId', invalid: '', valid: 'asset_2f183eda-5f01-4c34-93f6-47739c136bb2', select: true },
  { kind: 'video', field: 'video-duration', path: '/videos/0/durationSeconds', invalid: '0', valid: '181', select: false },
  { kind: 'moment', field: 'moment-asset', path: '/moments/0/assetId', invalid: '', valid: 'asset_2f183eda-5f01-4c34-93f6-47739c136bb2', select: true },
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
    const records = scenario.kind === 'video' ? source.videos : scenario.kind === 'event' ? source.events : source.moments
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
