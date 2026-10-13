import { readFile } from 'node:fs/promises'
import { expect, test, type Page } from '@playwright/test'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

/** 通过真实文件输入和防抖校验建立首页编辑上下文。 */
async function openHomepage(page: Page) {
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'homepage.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(fixture)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('homepage-open').click()
}

/** 从唯一保存源读取结果，不把临时表单误判为已应用快照。 */
async function snapshot(page: Page) {
  return JSON.parse(await page.getByRole('textbox', { name: 'JSON snapshot editor' }).inputValue()) as typeof fixture
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(/** 固定测试界面语言，不改变内容语言或 URL。 */ () => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/media/**', route => route.abort())
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
})

test('新增宣传位与板块编排，导出再导入保持内容', async ({ page }) => {
  await openHomepage(page)
  await page.getByTestId('homepage-hero-add').click()
  await page.getByTestId('homepage-headline-zh').fill('新专辑宣传')
  await page.getByTestId('homepage-headline-en').fill('New release')
  await page.getByTestId('homepage-hero-asset').selectOption('asset_hero_studio')
  await page.getByTestId('homepage-starts-at').fill('2026-08-28T00:00:00Z')
  await page.getByTestId('homepage-ends-at').fill('2026-09-01T00:00:00Z')
  const heroSection = page.getByTestId('homepage-section-section_hero')
  const heroOptions = heroSection.getByTestId('homepage-item-add-select')
  const newId = await page.getByTestId('homepage-hero-id').textContent()
  await heroOptions.selectOption(newId!.trim())
  await heroSection.getByTestId('homepage-item-add').click()
  await heroSection.getByTestId('homepage-item-up').last().click()
  await page.getByTestId('homepage-apply').click()
  await expect(page.getByTestId('homepage-error')).toHaveCount(0)
  await expect(page.getByTestId('homepage-apply')).toBeDisabled()
  const value = await snapshot(page)
  expect(value.heroSlides.at(-1)).toMatchObject({ headline: { 'zh-CN': '新专辑宣传', en: 'New release' }, startsAt: '2026-08-28T00:00:00Z' })
  expect(value.homepage.sections[0]!.itemIds).toEqual(['hero_studio', 'hero_stage', newId!.trim(), 'hero_release'])
  expect(value.releases).toEqual(fixture.releases)
  const downloadEvent = page.waitForEvent('download')
  await page.getByTestId('snapshot-export').click()
  const download = await downloadEvent
  const contents = await readFile((await download.path())!, 'utf8')
  expect(JSON.parse(contents)).toEqual(value)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'export.json', mimeType: 'application/json', buffer: Buffer.from(contents) })
  await page.getByTestId('snapshot-replace-discard').click()
  await expect(page.getByTestId('homepage-headline-zh')).toHaveCount(0)
  expect(await snapshot(page)).toEqual(value)
})

test('保留多表单输入，过期与隐藏目标拒绝覆盖', async ({ page }) => {
  await openHomepage(page)
  await page.getByTestId('homepage-headline-zh').fill('未应用首页')
  await expect(page.getByTestId('snapshot-export')).toBeDisabled()
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('release-title-en').fill('Applied music')
  await page.getByTestId('music-apply').click()
  await expect(page.getByTestId('homepage-apply')).toBeDisabled()
  await expect(page.getByTestId('homepage-headline-zh')).toHaveValue('未应用首页')
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('homepage-keep').click()
  await expect(page.getByTestId('homepage-headline-zh')).toHaveValue('未应用首页')
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('homepage-discard').click()
  const current = await snapshot(page)
  const section = page.getByTestId('homepage-section-section_music')
  await section.getByTestId('homepage-section-enabled').uncheck()
  await page.getByTestId('homepage-apply').click()
  await expect(page.getByTestId('homepage-error')).toContainText('hidden-target')
  expect(await snapshot(page)).toEqual(current)
  await section.getByTestId('homepage-section-enabled').check()
  await page.getByTestId('homepage-headline-en').fill('Keep this input')
  await page.locator('.rail-locale').click()
  await expect(page.getByTestId('homepage-headline-en')).toHaveValue('Keep this input')
  const close = page.getByTestId('homepage-close')
  await close.focus()
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('homepage-discard-prompt')).toBeVisible()
  expect(await page.evaluate(/** 检查真实布局，不依赖 CSS 断点声明。 */ () => document.documentElement.scrollWidth > innerWidth)).toBe(false)
})

test('保存期间锁定首页，修订冲突与成功均保持正确基线', async ({ page, baseURL }) => {
  await page.getByLabel('API base URL').fill(`${baseURL}/backend`)
  await page.locator('.connection-grid input[type="text"]').fill('ver_homepage')
  let fail = true
  let finish: (() => void) | undefined
  await page.route('**/backend/api/v1/versions/ver_homepage', async (route) => {
    let value = fixture
    let revision = 4
    if (route.request().method() === 'PUT') {
      expect(route.request().headers()['if-match']).toBe('"4"')
      if (fail) return route.fulfill({ status: 409, json: { code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_homepage' } })
      value = route.request().postDataJSON().snapshot
      revision = 5
      await new Promise<void>((resolve) => { finish = resolve })
    }
    await route.fulfill({ json: { id: 'ver_homepage', status: 'draft', revision, snapshot: value, checksum: 'sha256:contract' } })
  })
  await page.getByTestId('version-load').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('music-release-release_01').click()
  await page.getByTestId('homepage-headline-en').fill('Saved headline')
  await page.getByTestId('homepage-apply').click()
  await page.getByTestId('snapshot-save').click()
  await expect(page.locator('.notice--error')).toContainText('req_homepage')
  expect((await snapshot(page)).heroSlides[0]!.headline.en).toBe('Saved headline')
  fail = false
  await page.getByTestId('snapshot-save').click()
  await expect(page.getByTestId('homepage-headline-zh')).toBeDisabled()
  await expect(page.getByTestId('homepage-hero-add')).toBeDisabled()
  await expect.poll(() => Boolean(finish)).toBe(true)
  finish!()
  await expect(page.getByTestId('snapshot-unsaved')).toHaveCount(0)
  await expect(page.getByTestId('homepage-headline-zh')).toBeEnabled()
  await page.getByTestId('homepage-headline-en').fill('Next edit')
  await expect(page.getByTestId('homepage-apply')).toBeEnabled()
  await page.getByTestId('release-title-en').fill('Next music edit')
  await expect(page.getByTestId('music-apply')).toBeEnabled()
})
