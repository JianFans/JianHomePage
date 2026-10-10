import { readFile } from 'node:fs/promises'
import { resolve } from 'node:path'
import { expect, test, type Page } from '@playwright/test'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

/** 通过真实文件输入导入快照，并等待防抖分析完成。 */
async function importFixture(page: Page) {
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'snapshot.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(fixture)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
}

/** 从唯一 JSON 源读取结果，避免把表单显示值误认为已应用内容。 */
async function editorSnapshot(page: Page) {
  return JSON.parse(await page.getByRole('textbox', { name: 'JSON snapshot editor' }).inputValue()) as typeof fixture
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(/** 使用实际管理端偏好键固定测试语言。 */ () => {
    localStorage.setItem('yujian:admin-locale', 'en')
  })
  await page.route('**/media/**', async (route) => {
    const path = new URL(route.request().url()).pathname
    const asset = fixture.assets.find(item => item.src === path)
    if (!asset) return route.abort()
    await route.fulfill({ path: resolve('../web/public', path.slice(1)), contentType: asset.mimeType })
  })
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
})

test('创建作品、绑定素材、编排并导出可重新导入的快照', async ({ page }) => {
  await importFixture(page)
  await page.getByTestId('music-new').click()
  await page.getByTestId('release-title-zh').fill('浏览器作品')
  await page.getByTestId('release-title-en').fill('Browser release')
  await page.getByTestId('music-date').fill('2026-10-10')
  await page.getByTestId('music-kind').selectOption('ep')
  await page.getByTestId('music-cover').selectOption('asset_cover_01')
  await page.getByTestId('track-title-zh').fill('浏览器曲目')
  await page.getByTestId('track-duration').fill('180')
  await page.getByTestId('track-preview').selectOption('asset_preview_sample')
  await page.getByTestId('track-preview-duration').fill('3')
  const releaseLinks = page.locator('.music-editor > fieldset > details').first()
  await releaseLinks.locator('summary').click()
  await releaseLinks.getByTestId('link-add').click()
  await releaseLinks.getByTestId('link-url').fill('https://music.163.com/#/artist?id=12382128')
  await releaseLinks.getByTestId('link-provider').selectOption('netease-music')
  await page.getByTestId('section-include').check()
  await expect(page.getByTestId('section-section_music')).toContainText('Outside display limit')
  await page.getByTestId('section-up').click()
  await expect(page.getByTestId('section-section_music')).toContainText('Within display limit')
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('music-apply')).toBeDisabled()
  const snapshot = await editorSnapshot(page)
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  const release = snapshot.releases.at(-1)!
  expect(release.title).toEqual({ 'zh-CN': '浏览器作品', en: 'Browser release' })
  expect(release.kind).toBe('ep')
  expect(snapshot.tracks.at(-1)).toMatchObject({ releaseId: release.id, previewAssetId: 'asset_preview_sample', previewDurationSeconds: 3 })
  const downloadPromise = page.waitForEvent('download')
  await page.getByTestId('snapshot-export').click()
  const download = await downloadPromise
  const contents = await readFile((await download.path())!, 'utf8')
  expect(JSON.parse(contents)).toEqual(snapshot)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: download.suggestedFilename(), mimeType: 'application/json', buffer: Buffer.from(contents) })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.getByTestId(`music-release-${release.id}`)).toBeVisible()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  expect(await editorSnapshot(page)).toEqual(snapshot)
})

test('合法重复引用可编辑、排序并按引用移除，曲目记录保持唯一', async ({ page }) => {
  const value = structuredClone(fixture)
  value.releases[0]!.trackIds.push('track_01')
  value.homepage.sections.find(section => section.type === 'music')!.itemIds.push('release_02')
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'repeated.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(value)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('music-release-release_01').click()
  const tracks = page.getByTestId('track-editor')
  await expect(tracks).toHaveCount(2)
  await tracks.nth(1).getByTestId('track-title-zh').fill('重复引用的曲目')
  await expect(tracks.nth(0).getByTestId('track-title-zh')).toHaveValue('重复引用的曲目')
  await page.getByTestId('track-add').click()
  await tracks.nth(2).getByTestId('track-title-zh').fill('中间曲目')
  await tracks.nth(2).getByTestId('track-duration').fill('180')
  await tracks.nth(2).getByTestId('track-up').click()
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('music-error')).toHaveCount(0)
  await expect(page.getByTestId('music-apply')).toBeDisabled()
  const applied = await editorSnapshot(page)
  const newId = applied.tracks.at(-1)!.id
  expect(applied.releases[0]!.trackIds).toEqual(['track_01', newId, 'track_01'])
  expect(applied.homepage).toEqual(value.homepage)
  expect(applied.tracks.filter(track => track.id === 'track_01')).toHaveLength(1)
  expect(applied.tracks[0]!.title['zh-CN']).toBe('重复引用的曲目')
  await tracks.nth(0).getByTestId('track-remove').click()
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('music-apply')).toBeDisabled()
  const removed = await editorSnapshot(page)
  expect(removed.releases[0]!.trackIds).toEqual([newId, 'track_01'])
  expect(removed.tracks.filter(track => track.id === 'track_01')).toHaveLength(1)
  await expect(tracks.nth(1).getByTestId('track-title-zh')).toHaveValue('重复引用的曲目')
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
})

test('保护未应用输入、过期基线和内部目标引用', async ({ page }) => {
  await importFixture(page)
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('release-title-zh').fill('未应用')
  await page.getByTestId('music-release-release_02').click()
  await expect(page.getByTestId('music-discard-prompt')).toBeVisible()
  await page.getByTestId('music-keep').click()
  await expect(page.getByTestId('release-title-zh')).toHaveValue('未应用')
  await expect(page.getByTestId('snapshot-save')).toBeDisabled()
  const editor = page.getByRole('textbox', { name: 'JSON snapshot editor' })
  await editor.fill(`${await editor.inputValue()}\n`)
  await expect(page.getByText('Snapshot changed. Reload the release before applying.', { exact: true })).toBeVisible()
  await expect(page.getByTestId('music-apply')).toBeDisabled()
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('music-discard').click()
  await page.getByTestId('music-cover').selectOption('asset_cover_02')
  await page.getByTestId('music-apply').click()
  expect((await editorSnapshot(page)).releases[0]!.coverAssetId).toBe('asset_cover_02')
  const referenced = await editorSnapshot(page)
  referenced.heroSlides[0]!.target = { kind: 'internal', contentId: 'release_01' }
  await editor.fill(JSON.stringify(referenced))
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('section-include').uncheck()
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('music-error')).toContainText('hidden-target')
  expect(await editorSnapshot(page)).toEqual(referenced)
})

test('保存 API 契约：携带当前修订，冲突保留输入，保存期间锁定编辑', async ({ page, baseURL }) => {
  await page.getByLabel('API base URL').fill(`${baseURL}/backend`)
  await page.locator('.connection-grid input[type="text"]').fill('ver_browser')
  let revision = 3
  let conflict = true
  let saved = structuredClone(fixture)
  let finishSave: (() => void) | undefined
  await page.route('**/backend/api/v1/versions/ver_browser', async (route) => {
    if (route.request().method() === 'PUT') {
      expect(route.request().headers()['if-match']).toBe('"3"')
      if (conflict) return route.fulfill({ status: 409, json: { code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_browser' } })
      saved = route.request().postDataJSON().snapshot
      await new Promise<void>((resolve) => { finishSave = resolve })
      revision += 1
    }
    await route.fulfill({ json: { id: 'ver_browser', status: 'draft', revision, snapshot: saved, checksum: 'sha256:browser' } })
  })
  await page.getByTestId('version-load').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('release-title-en').fill('Saved title')
  await page.getByTestId('music-apply').click()
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.notice--error')).toContainText('req_browser')
  expect((await editorSnapshot(page)).releases[0]!.title.en).toBe('Saved title')
  conflict = false
  await page.getByTestId('snapshot-save').click()
  await expect(page.getByRole('textbox', { name: 'JSON snapshot editor' })).toBeDisabled()
  await expect(page.getByTestId('release-title-zh')).toBeDisabled()
  await expect(page.getByTestId('track-add')).toBeDisabled()
  await expect(page.getByTestId('version-load')).toBeDisabled()
  await expect.poll(() => Boolean(finishSave)).toBe(true)
  finishSave!()
  await expect(page.getByTestId('snapshot-unsaved')).toHaveCount(0)
  await expect(page.getByTestId('snapshot-save')).toBeEnabled()
  expect(saved.releases[0]!.title.en).toBe('Saved title')
})

test('键盘可操作，双语切换和窄屏布局保持内容', async ({ page }, testInfo) => {
  await importFixture(page)
  const release = page.getByTestId('music-release-release_01')
  await release.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('release-title-zh')).toBeVisible()
  await page.getByTestId('release-title-en').fill('Keep me')
  await page.locator('.rail-locale').click()
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('内容工作台')
  await expect(page.getByTestId('release-title-en')).toHaveValue('Keep me')
  const overflow = await page.evaluate(/** 比较真实布局宽度，不以 CSS 断点声明替代窄屏验证。 */ () => document.documentElement.scrollWidth > window.innerWidth)
  expect(overflow).toBe(false)
  await expect(page.getByTestId('music-apply')).toHaveAccessibleName('应用到快照')
  await page.screenshot({ path: testInfo.outputPath('music-workbench.png'), fullPage: true })
})
