import { expect, test } from '@playwright/test'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }
import { apply, navigate, roundTrip, snapshot, startStudio } from './studio'

test('新增现场、首页引用、真实预览与导出再导入', async ({ page }) => {
  await startStudio(page)
  await navigate(page, 'event')
  await page.getByTestId('event-add').click()
  for (const [id, value] of [['event-title-zh', '浏览器现场'], ['event-title-en', 'Browser event'], ['event-city-zh', '上海'], ['event-city-en', 'Shanghai'], ['event-venue-zh', '测试场馆'], ['event-venue-en', 'Test venue'], ['event-datetime', '2030-01-02T20:00:00+08:00'], ['event-detail-url', 'https://example.com/event']] as const) await page.getByTestId(id).fill(value)
  await page.getByTestId('event-poster').selectOption('asset_cover_01')
  await apply(page)
  const applied = await snapshot(page)
  const event = applied.events.at(-1)!
  expect(event).toMatchObject({ title: { 'zh-CN': '浏览器现场', en: 'Browser event' }, dateTime: '2030-01-02T20:00:00+08:00', city: { en: 'Shanghai' }, venue: { en: 'Test venue' }, status: 'scheduled', detailUrl: 'https://example.com/event', posterAssetId: 'asset_cover_01' })
  expect(applied.homepage).toEqual(fixture.homepage)
  await navigate(page, 'homepage')
  const section = page.getByTestId('homepage-section-section_event')
  await section.getByTestId('homepage-item-add-select').selectOption(event.id)
  await section.getByTestId('homepage-item-add').click()
  await section.getByTestId('homepage-section-limit').fill('3')
  await page.getByTestId('studio-preview').click()
  await expect(page.getByTestId('preview-frame').contentFrame().getByText('Browser event', { exact: true })).toBeVisible()
  await page.getByTestId('dialog-close').click()
  await roundTrip(page, await snapshot(page))
})

test('现场离开保护与无时区时间拒绝保存', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'event')
  await page.getByTestId('event-title-en').fill('Keep event')
  await navigate(page, 'video')
  await page.getByTestId('leave-keep').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('event-title-en')).toHaveValue('Keep event')
  await page.getByTestId('event-datetime').fill('2030-01-02T20:00:00')
  await page.getByTestId('snapshot-save').click()
  await expect(page.getByTestId('toast-viewport')).toContainText('/events/0/dateTime')
  await expect(page.getByTestId('event-error')).toHaveCount(0)
  expect(api.saves).toHaveLength(0)
  await page.locator('.rail-locale').filter({ visible: true }).click()
  await expect(page.getByTestId('event-title-en')).toHaveValue('Keep event')
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await navigate(page, 'video')
  await page.getByTestId('leave-discard').click()
  await navigate(page, 'event')
  await expect(page.getByTestId('event-title-en')).toHaveValue(fixture.events[0]!.title.en)
})

test('保存契约：409 保留、保存锁定、成功刷新现场基线', async ({ page }) => {
  const api = await startStudio(page)
  await navigate(page, 'event')
  api.conflict = true
  await page.getByTestId('event-title-en').fill('Saved event')
  await page.getByTestId('snapshot-save').click()
  await expect(page.getByTestId('toast-viewport')).toContainText('req_browser')
  expect((await snapshot(page)).events[0]!.title.en).toBe('Saved event')
  await page.getByTestId('event-title-en').fill('Retry event')
  api.conflict = false
  api.hold = true
  await page.getByTestId('snapshot-save').click()
  for (const id of ['event-title-zh', 'event-add', 'version-clone', 'studio-tools']) await expect(page.getByTestId(id)).toBeDisabled()
  await expect.poll(() => Boolean(api.finish)).toBe(true)
  api.finish!()
  await expect(page.getByTestId('event-title-en')).toHaveValue('Retry event')
  await expect(page.getByTestId('event-title-en')).toBeEnabled()
  await expect(page.locator('.studio-meta')).toContainText('Saved')
})

test('售罄、取消、过去与等于快照时间的现场不进入真实预览', async ({ page }) => {
  await startStudio(page)
  await navigate(page, 'event')
  const frame = page.getByTestId('preview-frame').contentFrame()
  await page.getByTestId('studio-preview').click()
  await expect(frame.getByText(fixture.events[0]!.title.en, { exact: true })).toBeVisible()
  await page.getByTestId('dialog-close').click()
  for (const status of ['sold-out', 'cancelled']) {
    await page.getByTestId('event-status').selectOption(status)
    await page.getByTestId('studio-preview').click()
    await expect(frame.getByText(fixture.events[1]!.title.en, { exact: true })).toBeVisible()
    await expect(frame.getByText(fixture.events[0]!.title.en, { exact: true })).toHaveCount(0)
    await page.getByTestId('dialog-close').click()
    expect((await snapshot(page)).events[0]!.status).toBe(status)
  }
  await page.getByTestId('event-status').selectOption('scheduled')
  for (const value of ['2000-01-01T00:00:00Z', fixture.generatedAt]) {
    await page.getByTestId('event-datetime').fill(value)
    await page.getByTestId('studio-preview').click()
    await expect(frame.getByText(fixture.events[1]!.title.en, { exact: true })).toBeVisible()
    await expect(frame.getByText(fixture.events[0]!.title.en, { exact: true })).toHaveCount(0)
    await page.getByTestId('dialog-close').click()
  }
  await page.getByTestId('event-datetime').fill('2030-01-02T20:00:00-03:00')
  await page.getByTestId('studio-preview').click()
  await expect(frame.getByText(fixture.events[0]!.title.en, { exact: true })).toBeVisible()
  await page.getByTestId('dialog-close').click()
  expect((await snapshot(page)).events[0]!.dateTime).toBe('2030-01-02T20:00:00-03:00')
})
