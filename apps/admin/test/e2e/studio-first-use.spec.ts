import { expect, test } from '@playwright/test'
import { backend, csrfToken, navigate, snapshot } from './studio'

test('空库直接提供有效初始内容，预览与手动保存无需导入文件', async ({ page }) => {
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  const api = await backend(page)
  api.records.clear()
  let saved: unknown
  await page.route('**/api/v1/versions', async route => {
    if (route.request().method() !== 'POST') return route.fallback()
    expect(route.request().headers()['x-csrf-token']).toBe(csrfToken)
    const body = route.request().postDataJSON()
    saved = body.snapshot
    await route.fulfill({ json: {
      id: 'ver_66d6fa96-4b9c-4a0f-9d8a-5cbd7fed12c9', name: '初始版本', status: 'draft', revision: 1,
      checksum: 'sha256:initial', snapshot: body.snapshot,
    } })
  })
  await page.goto('/')
  await expect(page.getByTestId('studio-shell')).toBeVisible()
  await expect(page.getByTestId('snapshot-save')).toBeEnabled()
  expect(api.writes).toHaveLength(0)
  const value = await snapshot(page)
  expect(value.site.brand['zh-CN']).toBe('遇健我')
  expect(value.site.artistName['zh-CN']).toBe('王子健')
  expect(value.assets).toEqual([])
  expect(value.releases).toEqual([])
  expect(value.homepage.sections.map(section => section.type)).toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
  await navigate(page, 'homepage')
  await expect(page.getByTestId('homepage-hero-add')).toBeEnabled()
  await page.getByTestId('studio-preview').click()
  const frame = page.getByTestId('preview-frame').contentFrame()
  await expect(frame.getByText('Meet Jian').first()).toBeVisible()
  await expect(frame.locator('img[src="undefined"]')).toHaveCount(0)
  await page.getByTestId('dialog-close').click()
  await page.getByTestId('snapshot-save').click()
  await expect(page.getByTestId('studio-version-name')).toHaveText('初始版本')
  expect(saved).toEqual(value)
})
