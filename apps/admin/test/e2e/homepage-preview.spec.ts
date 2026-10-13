import { createHash } from 'node:crypto'
import { readFile } from 'node:fs/promises'
import { expect, test, type Frame, type Page, type Request } from '@playwright/test'
import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json' with { type: 'json' }

const channel = 'yujian-homepage-preview-v1'

declare global {
  interface Window {
    __previewAudio?: HTMLAudioElement[]
    __retainedPreviewMedia?: { audio: HTMLAudioElement; video: HTMLVideoElement }
  }
}
// Two black H.264 frames generated locally with FFmpeg; no external video service is needed.
const videoBytes = Buffer.from('AAAAJGZ0eXBpc29tAAACAGlzb21pc282aXNvMmF2YzFtcDQxAAAC7G1vb3YAAABsbXZoZAAAAAAAAAAAAAAAAAAAA+gAAAAAAAEAAAEAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAIAAAHvdHJhawAAAFx0a2hkAAAAAwAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAQAAAAAAQAAAAEAAAAAABi21kaWEAAAAgbWRoZAAAAAAAAAAAAAAAAAAAQAAAAAAAVcQAAAAAAC1oZGxyAAAAAAAAAAB2aWRlAAAAAAAAAAAAAAAAVmlkZW9IYW5kbGVyAAAAATZtaW5mAAAAFHZtaGQAAAABAAAAAAAAAAAAAAAkZGluZgAAABxkcmVmAAAAAAAAAAEAAAAMdXJsIAAAAAEAAAD2c3RibAAAAKpzdHNkAAAAAAAAAAEAAACaYXZjMQAAAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAQABAASAAAAEgAAAAAAAAAARVMYXZjNjIuMTUuMTAwIGxpYngyNjQAAAAAAAAAAAAAABj//wAAADRhdmNDAWQACv/hABdnZAAKrNlewEQAAAMABAAAAwAIPEiWWAEABmjr48siwP34+AAAAAAQcGFzcAAAAAEAAAABAAAAEHN0dHMAAAAAAAAAAAAAABBzdHNjAAAAAAAAAAAAAAAUc3RzegAAAAAAAAAAAAAAAAAAABBzdGNvAAAAAAAAAAAAAAAobXZleAAAACB0cmV4AAAAAAAAAAEAAAABAAAAAAAAAAAAAAAAAAAAYXVkdGEAAABZbWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAsaWxzdAAAACSpdG9vAAAAHGRhdGEAAAABAAAAAExhdmY2Mi41LjEwMAAAAHhtb29mAAAAEG1maGQAAAAAAAAAAQAAAGB0cmFmAAAAJHRmaGQAAAA5AAAAAQAAAAAAAAMQAABAAAAAAsUBAQAAAAAAFHRmZHQBAAAAAAAAAAAAAAAAAAAgdHJ1bgAAAgUAAAACAAAAgAIAAAAAAALFAAAADAAAAtltZGF0AAACrQYF//+p3EXpvebZSLeWLNgg2SPu73gyNjQgLSBjb3JlIDE2NSByMzIyMiBiMzU2MDVhIC0gSC4yNjQvTVBFRy00IEFWQyBjb2RlYyAtIENvcHlsZWZ0IDIwMDMtMjAyNSAtIGh0dHA6Ly93d3cudmlkZW9sYW4ub3JnL3gyNjQuaHRtbCAtIG9wdGlvbnM6IGNhYmFjPTEgcmVmPTMgZGVibG9jaz0xOjA6MCBhbmFseXNlPTB4MzoweDExMyBtZT1oZXggc3VibWU9NyBwc3k9MSBwc3lfcmQ9MS4wMDowLjAwIG1peGVkX3JlZj0xIG1lX3JhbmdlPTE2IGNocm9tYV9tZT0xIHRyZWxsaXM9MSA4eDhkY3Q9MSBjcW09MCBkZWFkem9uZT0yMSwxMSBmYXN0X3Bza2lwPTEgY2hyb21hX3FwX29mZnNldD0tMiB0aHJlYWRzPTEgbG9va2FoZWFkX3RocmVhZHM9MSBzbGljZWRfdGhyZWFkcz0wIG5yPTAgZGVjaW1hdGU9MSBpbnRlcmxhY2VkPTAgYmx1cmF5X2NvbXBhdD0wIGNvbnN0cmFpbmVkX2ludHJhPTAgYmZyYW1lcz0zIGJfcHlyYW1pZD0yIGJfYWRhcHQ9MSBiX2JpYXM9MCBkaXJlY3Q9MSB3ZWlnaHRiPTEgb3Blbl9nb3A9MCB3ZWlnaHRwPTIga2V5aW50PTI1MCBrZXlpbnRfbWluPTEgc2NlbmVjdXQ9NDAgaW50cmFfcmVmcmVzaD0wIHJjX2xvb2thaGVhZD00MCByYz1jcmYgbWJ0cmVlPTEgY3JmPTIzLjAgcWNvbXA9MC42MCBxcG1pbj0wIHFwbWF4PTY5IHFwc3RlcD00IGlwX3JhdGlvPTEuNDAgYXE9MToxLjAwAIAAAAAQZYiEABb//vfTP8yy7JolgQAAAAhBmiFsQV/+8AAAAENtZnJhAAAAK3RmcmEBAAAAAAAAAQAAAAAAAAABAAAAAAAAAAAAAAAAAAADEAEBAQAAABBtZnJvAAAAAAAAAEM=', 'base64')

/** Import through the real file input and await canonical validation before opening the preview. */
async function importSnapshot(page: Page, value: unknown = fixture) {
  await page.getByTestId('snapshot-file-input').setInputFiles({ name: 'preview.json', mimeType: 'application/json', buffer: Buffer.from(JSON.stringify(value)) })
  await expect(page.getByTestId('snapshot-validation')).toHaveText('Valid')
}

/** Await all six shared sections so a mounted but uninitialized iframe cannot satisfy the test. */
async function previewFrame(page: Page): Promise<Frame> {
  await expect(page.getByTestId('preview-frame').contentFrame().locator('[data-home-section]')).toHaveCount(6)
  return (await (await page.getByTestId('preview-frame').elementHandle())!.contentFrame())!
}

/** Send from the real parent and flush the child's message queue without timing sleeps. */
async function postToPreview(page: Page, frame: Frame, data: unknown) {
  await frame.evaluate(() => {
    window.addEventListener('message', function barrier(event) {
      if (event.data?.e2eBarrier && event.ports[0]) {
        window.removeEventListener('message', barrier)
        event.ports[0].postMessage('done')
      }
    })
  })
  await page.evaluate(async (message) => {
    const target = document.querySelector<HTMLIFrameElement>('[data-testid="preview-frame"]')!.contentWindow!
    await new Promise<void>((resolve) => {
      const barrier = new MessageChannel()
      barrier.port1.onmessage = () => { barrier.port1.close(); resolve() }
      target.postMessage(message, location.origin)
      target.postMessage({ e2eBarrier: true }, location.origin, [barrier.port2])
    })
  }, data)
}

/** Check actual scaled geometry and the host document, including the phone project. */
async function expectContainedPreview(page: Page, frame: Frame, width: number) {
  expect(await frame.evaluate(() => innerWidth)).toBe(width)
  const geometry = await page.locator('.homepage-preview__viewport').evaluate((element) => {
    const viewport = element.getBoundingClientRect()
    const stage = element.parentElement!.getBoundingClientRect()
    return { width: viewport.width, left: viewport.left - stage.left, right: viewport.right - stage.right, overflow: document.documentElement.scrollWidth - innerWidth }
  })
  expect(geometry.width).toBeGreaterThan(0)
  expect(geometry.left).toBeGreaterThanOrEqual(-1)
  expect(geometry.right).toBeLessThanOrEqual(1)
  expect(geometry.overflow).toBeLessThanOrEqual(0)
}

/** Retain same-origin native media in the parent so detached execution contexts cannot hide leaks. */
async function retainMedia(page: Page) {
  await page.evaluate(() => {
    const child = document.querySelector<HTMLIFrameElement>('[data-testid="preview-frame"]')!.contentWindow!
    window.__retainedPreviewMedia = { audio: child.__previewAudio![0]!, video: child.document.querySelector('video')! }
  })
}

/** Inspect the old native objects after their iframe is gone, rather than only checking the DOM. */
async function expectReleasedMedia(page: Page) {
  await expect.poll(() => page.evaluate(() => {
    const { audio, video } = window.__retainedPreviewMedia!
    // Detached documents retain historical currentSrc values even after native pause/clear/load.
    return { audioPaused: audio.paused, audioSrc: audio.getAttribute('src'), videoPaused: video.paused, videoSrc: video.getAttribute('src') }
  })).toEqual({ audioPaused: true, audioSrc: '', videoPaused: true, videoSrc: null })
}

/** Build a canonical video variant with bytes and checksum matching the locally served asset. */
function videoSnapshot() {
  const value = structuredClone(fixture) as unknown as YujianContentSnapshot
  value.assets.push({ ...value.assets[0]!, id: 'asset_e2e_video', kind: 'video', src: '/media/e2e-video.mp4', mimeType: 'video/mp4', width: 16, height: 16, byteSize: videoBytes.length, checksum: `sha256:${createHash('sha256').update(videoBytes).digest('hex')}` })
  Object.assign(value.heroSlides[0]!, { mediaKind: 'video', assetId: 'asset_e2e_video', autoplay: true })
  return value
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(/** Only the editor locale is seeded; preview preferences must stay in memory. */ () => localStorage.setItem('yujian:admin-locale', 'en'))
  await page.route('**/media/**', route => route.abort())
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
})

test('真实握手、样式隔离、缩放视口和双向语言切换，关闭后重建文档', async ({ page }) => {
  await expect(page.getByTestId('preview-open')).toBeVisible()
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  await importSnapshot(page)
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  const editor = page.getByRole('textbox', { name: 'JSON snapshot editor' })
  const original = await editor.inputValue()
  const url = page.url()
  await page.getByTestId('preview-open').click()
  const frame = await previewFrame(page)
  expect(await frame.locator('[data-home-section]').evaluateAll(elements => elements.map(element => element.getAttribute('data-home-section')))).toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
  expect(await page.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--color-bg').trim())).toBe('')
  expect(await frame.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--bg').trim())).toBe('')
  expect(await frame.evaluate(() => getComputedStyle(document.documentElement).getPropertyValue('--color-bg').trim())).toBe('#0d0f11')
  await expectContainedPreview(page, frame, 1280)
  await page.getByTestId('preview-mobile').click()
  await expect.poll(() => frame.evaluate(() => innerWidth)).toBe(390)
  await expectContainedPreview(page, frame, 390)
  await page.getByTestId('preview-desktop').click()
  await expect.poll(() => frame.evaluate(() => innerWidth)).toBe(1280)
  await expectContainedPreview(page, frame, 1280)
  await expect(frame.locator('html')).toHaveAttribute('lang', 'en')
  await page.getByTestId('preview-language').click()
  await expect(frame.locator('html')).toHaveAttribute('lang', 'zh-CN')
  await frame.getByRole('button', { name: 'Switch to English', exact: true }).click()
  await expect(frame.locator('html')).toHaveAttribute('lang', 'en')
  await expect(page.getByTestId('preview-language')).toHaveText('EN')
  await expect(page.getByRole('heading', { level: 1 })).toHaveText('Content workspace')
  await expect(editor).toHaveValue(original)
  expect(page.url()).toBe(url)
  expect(frame.url()).toBe(`${url}preview`)
  expect(await page.evaluate(() => ({ ...localStorage }))).toEqual({ 'yujian:admin-locale': 'en' })
  await page.getByTestId('preview-close').click()
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  expect(frame.isDetached()).toBe(true)
  await page.locator('.rail-locale').click()
  await page.getByTestId('preview-open').click()
  const reopened = await previewFrame(page)
  expect(reopened).not.toBe(frame)
  await expect(reopened.locator('html')).toHaveAttribute('lang', 'zh-CN')
  await expect(page.getByRole('textbox', { name: 'JSON 快照编辑器' })).toHaveValue(original)
})

test('只预览已应用表单，非法 JSON 立即撤下旧内容并在恢复后重建', async ({ page }) => {
  const writes: string[] = []
  page.on('request', request => { if (request.method() !== 'GET' && request.method() !== 'HEAD') writes.push(request.url()) })
  await importSnapshot(page)
  await page.getByTestId('preview-open').click()
  const frame = await previewFrame(page)
  await page.getByTestId('homepage-open').click()
  await page.getByTestId('homepage-headline-en').fill('Applied browser headline')
  await expect(page.getByTestId('preview-form-dirty')).toBeVisible()
  await expect(frame.locator('.hero-headline')).toHaveText(fixture.heroSlides[0]!.headline.en)
  await page.getByTestId('homepage-apply').click()
  const applied = await previewFrame(page)
  await expect(applied.locator('.hero-headline')).toHaveText('Applied browser headline')
  expect(applied).not.toBe(frame)
  await expect(page.getByTestId('preview-form-dirty')).toHaveCount(0)
  const editor = page.getByRole('textbox', { name: 'JSON snapshot editor' })
  const valid = await editor.inputValue()
  await editor.fill('{')
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  await expect(page.getByTestId('preview-invalid')).toBeVisible()
  expect(applied.isDetached()).toBe(true)
  await editor.fill(valid)
  const restored = await previewFrame(page)
  expect(restored).not.toBe(applied)
  await expect(restored.locator('.hero-headline')).toHaveText('Applied browser headline')
  expect(writes).toEqual([])
})

test('拒绝错误来源、语言和 canonical 消息，阻止外链而保留内部锚点', async ({ page, context }) => {
  await importSnapshot(page)
  await page.getByTestId('preview-open').click()
  const frame = await previewFrame(page)
  await frame.evaluate((messageChannel) => window.postMessage({ channel: messageChannel, type: 'locale', locale: 'zh-CN' }, location.origin), channel)
  await page.evaluate((messageChannel) => window.postMessage({ channel: messageChannel, type: 'locale', locale: 'zh-CN' }, location.origin), channel)
  await page.evaluate(() => {
    const sibling = document.createElement('iframe')
    sibling.name = 'untrusted-sibling'
    sibling.hidden = true
    document.body.append(sibling)
  })
  const sibling = page.frames().find(candidate => candidate.name() === 'untrusted-sibling')!
  await sibling.evaluate((messageChannel) => {
    const target = parent.document.querySelector<HTMLIFrameElement>('[data-testid="preview-frame"]')!.contentWindow!
    target.postMessage({ channel: messageChannel, type: 'locale', locale: 'zh-CN' }, parent.location.origin)
    parent.postMessage({ channel: messageChannel, type: 'locale', locale: 'zh-CN' }, parent.location.origin)
  }, channel)
  await postToPreview(page, frame, { channel, type: 'locale', locale: 'fr' })
  await postToPreview(page, frame, { channel: 'other-channel', type: 'locale', locale: 'zh-CN' })
  await postToPreview(page, frame, { channel, type: 'snapshot', locale: 'zh-CN', snapshot: { ...fixture, schemaVersion: 'invalid' } })
  await postToPreview(page, frame, { channel, type: 'snapshot', locale: 'zh-CN', snapshot: { ...fixture, site: { ...fixture.site, canonicalUrl: 'javascript:alert(1)' } } })
  await page.evaluate((messageChannel) => {
    const target = document.querySelector<HTMLIFrameElement>('[data-testid="preview-frame"]')!.contentWindow!
    // Origin is browser-owned in postMessage; dispatch only this otherwise unforgeable mismatch.
    target.dispatchEvent(new MessageEvent('message', { origin: 'https://untrusted.invalid', source: window, data: { channel: messageChannel, type: 'locale', locale: 'zh-CN' } }))
  }, channel)
  await expect(frame.locator('html')).toHaveAttribute('lang', 'en')
  await expect(frame.locator('.hero-headline')).toHaveText(fixture.heroSlides[0]!.headline.en)
  await expect(page.getByTestId('preview-language')).toHaveText('EN')
  const url = frame.url()
  const external = frame.locator('a[href^="https:"]').first()
  await external.click()
  await external.click({ button: 'middle' })
  await postToPreview(page, frame, { channel, type: 'locale', locale: 'en' })
  expect(context.pages()).toHaveLength(1)
  expect(frame.url()).toBe(url)
  await frame.locator('.site-header a[href="#music"]').click()
  await expect.poll(() => new URL(frame.url()).hash).toBe('#music')
  await expect(frame.locator('#music')).toBeInViewport()
})

test('直接打开预览只等待，任意自发快照消息不能导入或请求内容 API', async ({ page }) => {
  const contentRequests: string[] = []
  page.on('request', request => { if (/\/api\/|homepage\.json|content\/fixtures/.test(request.url())) contentRequests.push(request.url()) })
  await page.goto('/preview')
  await expect(page.getByRole('status')).toHaveText('等待预览内容…')
  await page.evaluate(async (snapshot) => {
    await new Promise<void>((resolve) => {
      window.addEventListener('message', function barrier(event) {
        if (event.data?.e2eBarrier) { window.removeEventListener('message', barrier); resolve() }
      })
      window.postMessage({ channel: 'yujian-homepage-preview-v1', type: 'snapshot', snapshot, locale: 'en' }, location.origin)
      window.postMessage({ e2eBarrier: true }, location.origin)
    })
  }, fixture)
  await expect(page.locator('[data-home-section]')).toHaveCount(0)
  await expect(page.getByRole('status')).toHaveText('等待预览内容…')
  expect(contentRequests).toEqual([])
})

test('视频禁止自动播放；原生音视频在快照更新和关闭时暂停并清空来源', async ({ page }) => {
  const audioBytes = await readFile(new URL('../../../web/public/media/preview-sample.wav', import.meta.url))
  const mediaRequests: { url: string; authorization?: string }[] = []
  const pendingMedia = new Set<Request>()
  page.on('request', (request) => { if (/\/(?:e2e-video\.mp4|preview-sample\.wav)$/.test(request.url())) pendingMedia.add(request) })
  page.on('requestfinished', request => pendingMedia.delete(request))
  page.on('requestfailed', request => pendingMedia.delete(request))
  await page.route('**/media/e2e-video.mp4', (route) => {
    mediaRequests.push({ url: route.request().url(), authorization: route.request().headers().authorization })
    return route.fulfill({ contentType: 'video/mp4', body: videoBytes, headers: { 'access-control-allow-origin': '*' } })
  })
  await page.route('**/media/preview-sample.wav', (route) => {
    mediaRequests.push({ url: route.request().url(), authorization: route.request().headers().authorization })
    return route.fulfill({ contentType: 'audio/wav', body: audioBytes, headers: { 'access-control-allow-origin': '*' } })
  })
  await page.addInitScript(() => {
    const captured: HTMLAudioElement[] = []
    Object.defineProperty(window, '__previewAudio', { value: captured })
    // Preserve every native Audio behavior; retain references only for post-teardown assertions.
    window.Audio = new Proxy(window.Audio, { construct(target, args) { const audio = Reflect.construct(target, args) as HTMLAudioElement; captured.push(audio); return audio } })
  })
  await page.getByLabel('Session token').fill('preview-must-not-receive-this-token')
  await importSnapshot(page, videoSnapshot())
  await page.getByTestId('preview-open').click()
  const frame = await previewFrame(page)
  const video = frame.locator('video')
  expect(await video.evaluate(element => (element as HTMLVideoElement).autoplay)).toBe(false)
  expect(await video.evaluate(element => (element as HTMLVideoElement).paused)).toBe(true)
  await frame.getByRole('button', { name: 'Play video', exact: true }).click()
  await expect.poll(() => video.evaluate(element => (element as HTMLVideoElement).paused)).toBe(false)
  await frame.getByTestId('preview-trigger').first().click()
  await expect(frame.getByRole('button', { name: 'Pause preview', exact: true })).toBeVisible()
  expect(await frame.evaluate(() => window.__previewAudio![0]!.paused)).toBe(false)
  await retainMedia(page)
  const editor = page.getByRole('textbox', { name: 'JSON snapshot editor' })
  await editor.fill(`${await editor.inputValue()}\n`)
  await previewFrame(page)
  await expectReleasedMedia(page)
  await expect.poll(() => pendingMedia.size).toBe(0)
  const updated = await previewFrame(page)
  await updated.getByTestId('preview-trigger').first().click()
  await expect(updated.getByRole('button', { name: 'Pause preview', exact: true })).toBeVisible()
  await retainMedia(page)
  await page.getByTestId('preview-close').click()
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  await expectReleasedMedia(page)
  // This observes only the finite local fixtures; native object assertions prove media was released.
  await expect.poll(() => pendingMedia.size).toBe(0)
  expect(mediaRequests.length).toBeGreaterThan(0)
  expect(mediaRequests.every(request => request.url.startsWith('https://yujian.me/media/') && !request.authorization)).toBe(true)
})

test('相邻宣传位复用视频素材时，切换后原生视频保持禁自动播放', async ({ page }) => {
  await page.route('**/media/e2e-video.mp4', route => route.fulfill({ contentType: 'video/mp4', body: videoBytes, headers: { 'access-control-allow-origin': '*' } }))
  const value = videoSnapshot()
  Object.assign(value.heroSlides[1]!, { mediaKind: 'video', assetId: 'asset_e2e_video', autoplay: true })
  await importSnapshot(page, value)
  await page.getByTestId('preview-open').click()
  const frame = await previewFrame(page)
  await frame.getByRole('button', { name: 'Play video', exact: true }).click()
  await expect.poll(() => frame.locator('video').evaluate(element => (element as HTMLVideoElement).paused)).toBe(false)
  await frame.getByTestId('hero-next').click()
  await expect(frame.locator('.hero-headline')).toHaveText(value.heroSlides[1]!.headline.en!)
  await expect(frame.getByRole('button', { name: 'Play video', exact: true })).toBeVisible()
  await expect.poll(() => frame.locator('video').evaluate(element => (element as HTMLVideoElement).paused)).toBe(true)
})

test('未就绪文档超时后可重试并完成真实握手', async ({ page }) => {
  test.setTimeout(45_000)
  await page.route('**/preview', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Unresponsive preview</title>' }))
  await importSnapshot(page)
  await page.getByTestId('preview-open').click()
  await expect(page.getByTestId('preview-waiting')).toBeVisible()
  await expect(page.getByTestId('preview-error')).toBeVisible({ timeout: 15_000 })
  await expect(page.getByTestId('preview-frame')).toHaveCount(0)
  await page.unroute('**/preview')
  await page.getByTestId('preview-retry').click()
  await previewFrame(page)
  await expect(page.getByTestId('preview-error')).toHaveCount(0)
})
