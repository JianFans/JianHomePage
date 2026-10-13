import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

/** 导入真实 fixture 并等待共享分析。 */
async function openVideo(page: Page) {
  const source = structuredClone(fixture)
  source.assets.push({ ...source.assets[0]!, id: 'asset_video_test', kind: 'video', mimeType: 'video/mp4', src: '/media/test.mp4' })
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'video.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(source)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('video-open').click()
}
/** 读取唯一 JSON 源验证实际应用结果。 */
async function snapshot(page: Page) { return JSON.parse(await page.getByRole('textbox', { name: 'JSON snapshot editor' }).inputValue()) as typeof fixture }

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/media/**', route => route.abort())
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
})

test('新增影像、首页引用、共享预览与导出再导入', async ({ page }) => {
  await openVideo(page)
  await page.getByTestId('video-add').click()
  await page.getByTestId('video-title-zh').fill('浏览器影像')
  await page.getByTestId('video-title-en').fill('Browser video')
  await page.getByTestId('video-poster').selectOption('asset_cover_01')
  const media = { id: 'asset_video_test' }
  await page.getByTestId('video-asset').selectOption(media.id)
  await page.getByTestId('video-duration').fill('181')
  await page.getByTestId('video-featured').check()
  const links = page.getByTestId('video-fields').locator('details')
  await links.locator('summary').click()
  await links.getByTestId('link-url').fill('https://www.bilibili.com/video/BV123456')
  await page.getByTestId('video-apply').click()
  await expect(page.getByTestId('video-apply')).toBeDisabled()
  await expect(page.getByTestId('video-error')).toHaveCount(0)
  const applied = await snapshot(page)
  const video = applied.videos.at(-1)!
  expect(video).toMatchObject({ title: { 'zh-CN': '浏览器影像', en: 'Browser video' }, posterAssetId: 'asset_cover_01', videoAssetId: media.id, durationSeconds: 181, featured: true })
  expect(applied.homepage).toEqual(fixture.homepage)
  await page.getByTestId('homepage-open').click()
  const section = page.getByTestId('homepage-section-section_video')
  await section.getByTestId('homepage-item-add-select').selectOption(video.id)
  await section.getByTestId('homepage-item-add').click()
  await section.getByTestId('homepage-section-limit').fill('5')
  await page.getByTestId('homepage-apply').click()
  await expect(page.getByTestId('homepage-error')).toHaveCount(0)
  await page.getByTestId('preview-open').click()
  await expect(page.frameLocator('iframe').getByText('Browser video', { exact: true })).toBeVisible()
  const final = await snapshot(page)
  const downloadEvent = page.waitForEvent('download')
  await page.getByTestId('snapshot-export').click()
  const contents = await readFile((await (await downloadEvent).path())!, 'utf8')
  expect(JSON.parse(contents)).toEqual(final)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'export.json', mimeType: 'application/json', buffer: Buffer.from(contents) })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.getByTestId('video-fields')).toHaveCount(0)
  expect(await snapshot(page)).toEqual(final)
})

test('三个工作台保留各自输入，应用后阻止过期覆盖与失败替换丢失', async ({ page }) => {
  await openVideo(page)
  await page.getByTestId('video-title-zh').fill('影像待处理')
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('homepage-headline-zh').fill('首页待处理')
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('release-title-en').fill('Applied music')
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('video-title-zh')).toHaveValue('影像待处理')
  await expect(page.getByTestId('homepage-headline-zh')).toHaveValue('首页待处理')
  await expect(page.getByTestId('video-apply')).toBeDisabled()
  await expect(page.getByTestId('homepage-apply')).toBeDisabled()
  await expect(page.getByTestId('snapshot-export')).toBeDisabled()
  await page.getByTestId('video-open').click()
  await page.getByTestId('video-keep').click()
  await expect(page.getByTestId('video-title-zh')).toHaveValue('影像待处理')
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'bad.txt', mimeType: 'text/plain', buffer: Buffer.from('{}') })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.locator('.notice--error')).toContainText('JSON file')
  await expect(page.getByTestId('video-title-zh')).toHaveValue('影像待处理')
  await page.getByTestId('video-open').click()
  await page.getByTestId('video-discard').click()
  await expect(page.getByTestId('video-title-zh')).toHaveValue(fixture.videos[0]!.title['zh-CN'])
  await page.getByRole('textbox', { name: 'JSON snapshot editor' }).fill('{')
  await expect(page.getByTestId('video-apply')).toBeDisabled()
  await expect(page.getByTestId('video-title-zh')).toHaveValue(fixture.videos[0]!.title['zh-CN'])
})

test('双语与键盘编辑保留内容且窄屏无溢出', async ({ page }, testInfo) => {
  await openVideo(page)
  await page.getByTestId('video-title-en').fill('Keep video')
  await page.locator('.rail-locale').click()
  await expect(page.getByTestId('video-title-en')).toHaveValue('Keep video')
  await expect(page.getByTestId('video-apply')).toHaveAccessibleName('应用到快照')
  await page.getByTestId('video-close').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('video-discard-prompt')).toBeVisible()
  await page.getByTestId('video-keep').focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('video-title-en')).toHaveValue('Keep video')
  expect(await page.evaluate(/** 验证真实布局宽度。 */ () => document.documentElement.scrollWidth > innerWidth)).toBe(false)
  await page.screenshot({ path: testInfo.outputPath('video-workbench.png'), fullPage: true })
})

test('保存契约：409 保留、保存锁定、成功刷新全部已打开基线', async ({ page, baseURL }) => {
  await page.getByLabel('API base URL').fill(`${baseURL}/backend`)
  await page.locator('.connection-grid input[type="text"]').fill('ver_video')
  let fail = true
  let finish: (() => void) | undefined
  await page.route('**/backend/api/v1/versions/ver_video', async (route) => {
    let value = fixture
    let revision = 4
    if (route.request().method() === 'PUT') {
      expect(route.request().headers()['if-match']).toBe('"4"')
      if (fail) return route.fulfill({ status: 409, json: { code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_video' } })
      value = route.request().postDataJSON().snapshot
      revision = 5
      await new Promise<void>((resolve) => { finish = resolve })
    }
    await route.fulfill({ json: { id: 'ver_video', status: 'draft', revision, snapshot: value, checksum: 'sha256:contract' } })
  })
  await page.getByTestId('version-load').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('video-open').click()
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('video-title-en').fill('Saved video')
  await page.getByTestId('video-apply').click()
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.notice--error')).toContainText('req_video')
  expect((await snapshot(page)).videos[0]!.title.en).toBe('Saved video')
  await page.getByTestId('video-title-en').fill('Retry video')
  await expect(page.getByTestId('video-apply')).toBeEnabled()
  await page.getByTestId('video-apply').click()
  fail = false
  await page.getByTestId('snapshot-save').click()
  for (const id of ['video-title-zh', 'video-add', 'video-open', 'version-load', 'homepage-open', 'music-new']) await expect(page.getByTestId(id)).toBeDisabled()
  await expect.poll(() => Boolean(finish)).toBe(true)
  finish!()
  await expect(page.getByTestId('snapshot-unsaved')).toHaveCount(0)
  await page.getByTestId('video-title-en').fill('Next video')
  await page.getByTestId('homepage-headline-en').fill('Next homepage')
  await page.getByTestId('release-title-en').fill('Next music')
  for (const id of ['video-apply', 'homepage-apply', 'music-apply']) await expect(page.getByTestId(id)).toBeEnabled()
})
