import { readFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { expect, type Page, type Request } from '@playwright/test'
import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

// The canonical validator uses bundler JSON imports; require keeps them compatible with Node ESM.
const schema: typeof import('@yujian/schema') = createRequire(import.meta.url)('@yujian/schema')
const assertContentSnapshot: (value: unknown) => asserts value is YujianContentSnapshot = schema.assertContentSnapshot

export const csrfToken = 'csrf-browser-contract'
export type Snapshot = YujianContentSnapshot
export type Module = 'overview' | 'homepage' | 'music' | 'video' | 'event' | 'moment' | 'assets' | 'history'

export function fixtureSnapshot(): Snapshot {
  const value: unknown = structuredClone(fixture)
  assertContentSnapshot(value)
  return value
}

/** Mock only the HTTP boundary; session, navigation, forms and preview remain real. */
export async function backend(page: Page, value: Snapshot = fixtureSnapshot()) {
  const historical = fixtureSnapshot()
  historical.heroSlides[0]!.headline.en = 'Historical headline'
  const records = new Map([
    ['ver_14cea400-a42f-4560-9770-3de88cae9d5a', { id: 'ver_14cea400-a42f-4560-9770-3de88cae9d5a', name: 'Working draft', status: 'draft', revision: 4, checksum: 'sha256:browser', createdAt: '2026-10-14T00:00:00Z', updatedAt: '2026-10-14T00:00:00Z', snapshot: value }],
    ['ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', { id: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', name: 'Previous version', status: 'archived', revision: 2, checksum: 'sha256:history', createdAt: '2026-10-13T00:00:00Z', updatedAt: '2026-10-13T00:00:00Z', snapshot: historical }],
  ])
  const control = { conflict: false, hold: false, finish: undefined as (() => void) | undefined, saves: [] as Snapshot[], writes: [] as Request[], records }
  await page.route('**/api/v1/**', async (route) => {
    const request = route.request()
    const path = new URL(request.url()).pathname
    const method = request.method()
    if (method === 'OPTIONS') return route.fulfill({ status: 204 })
    if (path.endsWith('/auth/session')) return route.fulfill({ json: { authenticated: true, loginAvailable: true, principal: { subject: 'editor-browser', name: 'Browser editor', roles: ['admin'] }, csrfToken, expiresAt: new Date(Date.now() + 3_600_000).toISOString() } })
    if (!['GET', 'HEAD'].includes(method)) {
      control.writes.push(request)
      expect(request.headers()['x-csrf-token']).toBe(csrfToken)
      expect(request.headers().authorization).toBeUndefined()
    }
    if (path.endsWith('/auth/logout')) return route.fulfill({ status: 204 })
    if (path.endsWith('/assets')) return route.fulfill({ json: { items: [] } })
    if (path.endsWith('/versions')) return route.fulfill({ json: { items: [...records.values()].map((record) => {
      const { snapshot: content, ...summary } = record
      void content
      return summary
    }) } })
    const id = path.split('/').at(path.endsWith('/clone') ? -2 : -1)!
    const record = records.get(id)
    if (!record) return route.fulfill({ status: 404, json: { code: 'not_found', message: 'Not found', requestId: 'req_f0dcb841-b62c-49df-9dba-420465f2015d' } })
    if (path.endsWith('/clone')) {
      expect(request.postDataJSON()).toEqual({ name: 'Independent draft' })
      const cloned = { ...record, id: 'ver_9cb0468e-f117-476b-be75-eed979e0c719', name: request.postDataJSON().name, sourceVersionId: id, status: 'draft', revision: 1, snapshot: structuredClone(record.snapshot) }
      records.set(cloned.id, cloned)
      return route.fulfill({ json: cloned })
    }
    if (method === 'PUT') {
      expect(request.headers()['if-match']).toBe(`"${record.revision}"`)
      const next: unknown = request.postDataJSON().snapshot
      assertContentSnapshot(next)
      control.saves.push(next)
      if (control.conflict) return route.fulfill({ status: 409, json: { code: 'revision_conflict', message: 'Revision conflict', requestId: 'req_c4464811-558f-4491-b698-2404d74f0167' } })
      if (control.hold) await new Promise<void>((resolve) => { control.finish = resolve })
      record.snapshot = next
      record.revision++
    }
    return route.fulfill({ json: record })
  })
  return control
}

export async function startStudio(page: Page, value?: Snapshot) {
  await page.addInitScript(() => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/media/**', route => route.abort())
  const api = await backend(page, value)
  await page.goto('/')
  await expect(page.getByTestId('studio-shell')).toBeVisible()
  await expect(page.getByTestId('studio-version-name')).toHaveText('Working draft')
  await expect(page.getByTestId('studio-preview')).toBeEnabled()
  return api
}

/** Use the same visible drawer and module controls as phone and desktop users. */
export async function navigate(page: Page, module: Module) {
  if (await page.locator('.studio-menu').isVisible()) await page.locator('.studio-menu').click()
  await page.getByTestId(`nav-${module}`).filter({ visible: true }).click()
}

export async function openTools(page: Page) {
  await page.getByTestId('studio-tools').click()
  await expect(page.getByRole('textbox', { name: 'JSON snapshot editor' })).toBeVisible()
}

export async function snapshot(page: Page): Promise<Snapshot> {
  await openTools(page)
  const value: unknown = JSON.parse(await page.getByRole('textbox', { name: 'JSON snapshot editor' }).inputValue())
  assertContentSnapshot(value)
  await page.getByTestId('dialog-close').click()
  return value
}

export async function importSnapshot(page: Page, value: unknown = fixture) {
  await openTools(page)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'snapshot.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(value)) })
  if (await page.getByTestId('leave-dialog').isVisible()) await page.getByTestId('leave-discard').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('dialog-close').click()
}

/** Preview is the supported transaction for inspecting unsaved form content. */
export async function apply(page: Page) {
  await page.getByTestId('studio-preview').click()
  await expect(page.getByTestId('preview-frame').contentFrame().locator('[data-home-section]')).toHaveCount(6)
  await page.getByTestId('dialog-close').click()
}

export async function roundTrip(page: Page, value: Snapshot) {
  await openTools(page)
  const downloadEvent = page.waitForEvent('download')
  await page.getByTestId('snapshot-export').click()
  const contents = await readFile((await (await downloadEvent).path())!, 'utf8')
  expect(JSON.parse(contents)).toEqual(value)
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'export.json', mimeType: 'application/json', buffer: Buffer.from(contents) })
  if (await page.getByTestId('leave-dialog').isVisible()) await page.getByTestId('leave-discard').click()
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
  await page.getByTestId('dialog-close').click()
  expect(await snapshot(page)).toEqual(value)
}

export async function selectVersion(page: Page, id: string) {
  if (await page.locator('.studio-version-select').isVisible()) await page.locator('.studio-version-select').selectOption(id)
  else await page.getByTestId(`version-${id}`).click()
}
