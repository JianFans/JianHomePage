import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

/** 导入有效快照并打开现场工作台。 */
async function openEvent(page: Page) {
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'event.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(fixture)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('event-open').click()
}
/** 从唯一 JSON 源读取实际应用结果。 */
async function snapshot(page: Page) { return JSON.parse(await page.getByRole('textbox', { name: 'JSON snapshot editor' }).inputValue()) as typeof fixture }

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/media/**', route => route.abort())
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
})

test('新增现场、首页引用、真实预览与导出再导入', async ({ page }) => {
  await openEvent(page)
  await page.getByTestId('event-add').click()
  for (const [id, value] of [['event-title-zh', '浏览器现场'], ['event-title-en', 'Browser event'], ['event-city-zh', '上海'], ['event-city-en', 'Shanghai'], ['event-venue-zh', '测试场馆'], ['event-venue-en', 'Test venue'], ['event-datetime', '2030-01-02T20:00:00+08:00'], ['event-detail-url', 'https://example.com/event']] as const) await page.getByTestId(id).fill(value)
  await page.getByTestId('event-poster').selectOption('asset_cover_01')
  await page.getByTestId('event-apply').click()
  await expect(page.getByTestId('event-error')).toHaveCount(0)
  const applied = await snapshot(page)
  const event = applied.events.at(-1)!
  expect(event).toMatchObject({ title: { 'zh-CN': '浏览器现场', en: 'Browser event' }, dateTime: '2030-01-02T20:00:00+08:00', city: { en: 'Shanghai' }, venue: { en: 'Test venue' }, status: 'scheduled', detailUrl: 'https://example.com/event', posterAssetId: 'asset_cover_01' })
  expect(applied.homepage).toEqual(fixture.homepage)
  await page.getByTestId('homepage-open').click()
  const section = page.getByTestId('homepage-section-section_event')
  await section.getByTestId('homepage-item-add-select').selectOption(event.id)
  await section.getByTestId('homepage-item-add').click()
  await section.getByTestId('homepage-section-limit').fill('3')
  await page.getByTestId('homepage-apply').click()
  await expect(page.getByTestId('homepage-error')).toHaveCount(0)
  await page.getByTestId('preview-open').click()
  await expect(page.frameLocator('iframe').getByText('Browser event', { exact: true })).toBeVisible()
  const final = await snapshot(page)
  const downloadEvent = page.waitForEvent('download')
  await page.getByTestId('snapshot-export').click()
  const contents = await readFile((await (await downloadEvent).path())!, 'utf8')
  expect(JSON.parse(contents)).toEqual(final)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'export.json', mimeType: 'application/json', buffer: Buffer.from(contents) })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.getByTestId('event-fields')).toHaveCount(0)
  expect(await snapshot(page)).toEqual(final)
})

test('四个工作台保留各自输入，应用后阻止过期覆盖与失败替换丢失', async ({ page }) => {
  await openEvent(page)
  await page.getByTestId('event-title-zh').fill('现场待处理')
  await page.getByTestId('video-open').click()
  await page.getByTestId('video-title-zh').fill('影像待处理')
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('homepage-headline-zh').fill('首页待处理')
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('release-title-en').fill('Applied music')
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('event-title-zh')).toHaveValue('现场待处理')
  await expect(page.getByTestId('homepage-headline-zh')).toHaveValue('首页待处理')
  await expect(page.getByTestId('event-apply')).toBeDisabled()
  await expect(page.getByTestId('homepage-apply')).toBeDisabled()
  await expect(page.getByTestId('snapshot-export')).toBeDisabled()
  await page.getByTestId('event-open').click()
  await page.getByTestId('event-keep').click()
  await expect(page.getByTestId('event-title-zh')).toHaveValue('现场待处理')
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'bad.txt', mimeType: 'text/plain', buffer: Buffer.from('{}') })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.locator('.notice--error')).toContainText('JSON file')
  await expect(page.getByTestId('event-title-zh')).toHaveValue('现场待处理')
  await page.getByTestId('event-open').click()
  await page.getByTestId('event-discard').click()
  await expect(page.getByTestId('event-title-zh')).toHaveValue(fixture.events[0]!.title['zh-CN'])
  await page.getByRole('textbox', { name: 'JSON snapshot editor' }).fill('{')
  await expect(page.getByTestId('event-apply')).toBeDisabled()
  await expect(page.getByTestId('event-title-zh')).toHaveValue(fixture.events[0]!.title['zh-CN'])
})

test('双语与键盘编辑保留内容且窄屏无溢出', async ({ page }, testInfo) => {
  await openEvent(page)
  await page.getByTestId('event-title-en').fill('Keep event')
  await page.locator('.rail-locale').click()
  await expect(page.getByTestId('event-title-en')).toHaveValue('Keep event')
  await expect(page.getByTestId('event-apply')).toHaveAccessibleName('应用到快照')
  await page.getByTestId('event-close').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('event-discard-prompt')).toBeVisible()
  await page.getByTestId('event-keep').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('event-title-en')).toHaveValue('Keep event')
  expect(await page.evaluate(/** 验证真实布局宽度。 */ () => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('event-workbench.png'), fullPage: true })
})

test('保存契约：409 保留、保存锁定、成功刷新全部已打开基线', async ({ page, baseURL }) => {
  await page.getByLabel('API base URL').fill(`${baseURL}/backend`)
  await page.locator('.connection-grid input[type="text"]').fill('ver_event')
  let fail = true
  let finish: (() => void) | undefined
  await page.route('**/backend/api/v1/versions/ver_event', async (route) => {
    let value = fixture
    let revision = 4
    if (route.request().method() === 'PUT') {
      expect(route.request().headers()['if-match']).toBe('"4"')
      if (fail) return route.fulfill({ status: 409, json: { code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_event' } })
      value = route.request().postDataJSON().snapshot
      revision = 5
      await new Promise<void>((resolve) => { finish = resolve })
    }
    await route.fulfill({ json: { id: 'ver_event', status: 'draft', revision, snapshot: value, checksum: 'sha256:contract' } })
  })
  await page.getByTestId('version-load').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('event-open').click()
  await page.getByTestId('video-open').click()
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('event-title-en').fill('Saved event')
  await page.getByTestId('event-apply').click()
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.notice--error')).toContainText('req_event')
  expect((await snapshot(page)).events[0]!.title.en).toBe('Saved event')
  await page.getByTestId('event-title-en').fill('Retry event')
  await expect(page.getByTestId('event-apply')).toBeEnabled()
  await page.getByTestId('event-apply').click()
  fail = false
  await page.getByTestId('snapshot-save').click()
  for (const id of ['event-title-zh', 'event-add', 'event-open', 'version-load', 'homepage-open', 'music-new', 'video-open', 'video-add']) await expect(page.getByTestId(id)).toBeDisabled()
  await expect.poll(() => Boolean(finish)).toBe(true)
  finish!()
  await expect(page.getByTestId('snapshot-unsaved')).toHaveCount(0)
  await page.getByTestId('event-title-en').fill('Next event')
  await page.getByTestId('homepage-headline-en').fill('Next homepage')
  await page.getByTestId('release-title-en').fill('Next music')
  await page.getByTestId('video-title-en').fill('Next video')
  for (const id of ['event-apply', 'homepage-apply', 'music-apply', 'video-apply']) await expect(page.getByTestId(id)).toBeEnabled()
})

test('售罄、取消、过去与等于快照时间的现场不进入真实预览', async ({ page }) => {
  await openEvent(page)
  await page.getByTestId('preview-open').click()
  await expect(page.frameLocator('iframe').getByText(fixture.events[0]!.title.en, { exact: true })).toBeVisible()
  for (const status of ['sold-out', 'cancelled']) {
    await page.getByTestId('event-status').selectOption(status)
    await page.getByTestId('event-apply').click()
    await expect(page.getByTestId('event-error')).toHaveCount(0)
    await expect(page.frameLocator('iframe').getByText(fixture.events[1]!.title.en, { exact: true })).toBeVisible()
    await expect(page.frameLocator('iframe').getByText(fixture.events[0]!.title.en, { exact: true })).toHaveCount(0)
    expect((await snapshot(page)).events[0]!.status).toBe(status)
  }
  await page.getByTestId('event-status').selectOption('scheduled')
  for (const value of ['2000-01-01T00:00:00Z', fixture.generatedAt]) {
    await page.getByTestId('event-datetime').fill(value)
    await page.getByTestId('event-apply').click()
    await expect(page.getByTestId('event-error')).toHaveCount(0)
    await expect(page.frameLocator('iframe').getByText(fixture.events[1]!.title.en, { exact: true })).toBeVisible()
    await expect(page.frameLocator('iframe').getByText(fixture.events[0]!.title.en, { exact: true })).toHaveCount(0)
  }
  await page.getByTestId('event-datetime').fill('2030-01-02T20:00:00-03:00')
  await page.getByTestId('event-apply').click()
  await expect(page.frameLocator('iframe').getByText(fixture.events[0]!.title.en, { exact: true })).toBeVisible()
  expect((await snapshot(page)).events[0]!.dateTime).toBe('2030-01-02T20:00:00-03:00')
})
