import { expect, test } from '@playwright/test'

const sectionIds = ['music', 'video', 'event', 'moment', 'artist']
const clientHydrationTimeout = 15_000

async function openHomepage(page: import('@playwright/test').Page) {
  await page.goto('/')
  await expect(page.getByRole('heading', { level: 1 })).toContainText(/遇健我|Meet Jian/)
  await waitForLocaleHydration(page)
}

/** The shared standalone app persists its browser locale from onMounted on every route. */
async function waitForLocaleHydration(page: import('@playwright/test').Page) {
  await expect.poll(
    () => page.evaluate(() => localStorage.getItem('yujian:locale')),
    { timeout: clientHydrationTimeout },
  ).toMatch(/^(zh-CN|en)$/)
}

async function expectHeroPixels(page: import('@playwright/test').Page) {
  const image = page.locator('[data-testid="hero-media"] img')
  await expect(image).toBeVisible()
  await expect.poll(() => image.evaluate(element => (element as HTMLImageElement).naturalWidth)).toBeGreaterThan(0)

  const pixelRange = await image.evaluate((element) => {
    const imageElement = element as HTMLImageElement
    const canvas = document.createElement('canvas')
    canvas.width = 32
    canvas.height = 32
    const context = canvas.getContext('2d')
    if (!context) {
      return 0
    }
    context.drawImage(imageElement, 0, 0, canvas.width, canvas.height)
    const pixels = context.getImageData(0, 0, canvas.width, canvas.height).data
    let minimum = 255
    let maximum = 0
    for (let index = 0; index < pixels.length; index += 4) {
      const luminance = (pixels[index]! + pixels[index + 1]! + pixels[index + 2]!) / 3
      minimum = Math.min(minimum, luminance)
      maximum = Math.max(maximum, luminance)
    }
    return maximum - minimum
  })

  expect(pixelRange).toBeGreaterThan(8)
}

test.describe('桌面首页', () => {
  test.use({ viewport: { width: 1440, height: 1000 } })

  test('影像目录展示全部作品、稳定地址和双语页面标题', async ({ page }) => {
    await page.goto('/video/')
    await waitForLocaleHydration(page)

    await expect(page).toHaveTitle(/全部影像|All videos/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/全部影像|All videos/)
    await expect(page.locator('article.video-card')).toHaveCount(3)
    await expect(page.getByRole('navigation').getByRole('link', { name: /影像|Video/ })).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/video/')

    if (await page.locator('html').getAttribute('lang') !== 'en') {
      await page.getByRole('button', { name: /Switch to English|切换为中文/ }).click()
    }

    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('All videos')
    await expect(page.getByRole('button', { name: /Switch to Chinese|切换为中文/ })).toBeVisible()
  })

  test('现场目录展示完整演出列表、状态和稳定页面地址', async ({ page }) => {
    await page.goto('/events/')
    await waitForLocaleHydration(page)

    await expect(page).toHaveTitle(/全部现场|Live events/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/全部现场|Live events/)
    await expect(page.locator('[data-testid="event-card"]')).toHaveCount(2)
    await expect(page.getByRole('navigation').getByRole('link', { name: /现场|Live/ })).toHaveAttribute('aria-current', 'page')
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/events/')
    await expect(page.locator('[data-testid="event-card"] a').first()).toHaveAttribute('rel', 'noopener noreferrer')

    if (await page.locator('html').getAttribute('lang') !== 'en') {
      await page.getByRole('button', { name: /Switch to English|切换为中文/ }).click()
    }

    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    await expect(page.getByRole('heading', { level: 1 })).toHaveText('Live events')
  })

  test('片段目录入口可浏览发布快照中的全部片段', async ({ page }) => {
    await openHomepage(page)
    await page.getByRole('link', { name: /查看全部片段|View all moments/ }).click()

    await expect(page).toHaveURL(/\/moments\/$/)
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(/全部片段|Moments/)
    await expect(page.locator('[data-testid="moment-card"]')).toHaveCount(3)
    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me/moments/')
  })

  test('预渲染完整板块并保留首屏的下一段提示', async ({ page }, testInfo) => {
    await openHomepage(page)

    await expect(page.getByText(/王子健|Wang Zijian/).first()).toBeVisible()
    for (const sectionId of sectionIds) {
      await expect(page.locator(`#${sectionId}`)).toBeAttached()
    }
    await expect(page.getByTestId('music-more')).toHaveAttribute('href', '/music/')
    await expect(page.getByTestId('music-external')).toHaveAttribute(
      'href',
      'https://y.qq.com/n/ryqq_v2/singer/0036zydh4H05PB',
    )
    await expect(page.getByTestId('video-more')).toHaveAttribute('href', '/video/')
    await expect(page.locator('footer')).toContainText('© 2026')

    const edge = await page.locator('.next-section-edge').boundingBox()
    expect(edge).not.toBeNull()
    expect(edge!.y).toBeLessThan(1000)
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
    await expectHeroPixels(page)
    await page.screenshot({ path: testInfo.outputPath('home-desktop.png'), fullPage: true })
  })

  test('试听只创建一个 Dock，无试听作品仍保留平台入口', async ({ page }) => {
    const pageErrors: string[] = []
    page.on('pageerror', error => pageErrors.push(error.message))
    await openHomepage(page)
    const cards = page.getByTestId('music-card')

    await expect(cards).toHaveCount(5)
    await expect(cards.nth(1).getByTestId('preview-trigger')).toHaveCount(0)
    await expect(cards.nth(1).getByTestId('platform-links')).toBeVisible()

    await cards.first().getByTestId('preview-trigger').click()
    await page.waitForTimeout(250)
    const diagnostics = await page.evaluate(() => ({
      audioElements: document.querySelectorAll('audio').length,
      dockCount: document.querySelectorAll('[data-testid="audio-dock"]').length,
      hostChildren: document.querySelector('[data-testid="audio-dock-host"]')?.childElementCount ?? -1,
      pressed: document.querySelector('[data-testid="preview-trigger"]')?.getAttribute('aria-pressed'),
    }))
    expect(
      diagnostics.dockCount,
      JSON.stringify({ diagnostics, pageErrors }),
    ).toBe(1)
    await expect(page.getByTestId('audio-dock')).toBeVisible()

    const previous = page.getByRole('button', { name: /上一首|Previous track/ })
    const next = page.getByRole('button', { name: /下一首|Next track/ })
    await expect(previous).toBeDisabled()
    await expect(next).toBeEnabled()
    await next.click()
    await expect(page.getByTestId('audio-dock')).toContainText(/示例曲目 03|Sample Track 03/)
    await expect(previous).toBeEnabled()
  })

  test('图片加载失败时使用内嵌占位且保持媒体元素', async ({ page }) => {
    await openHomepage(page)
    const cover = page.getByTestId('music-card').first().locator('img')

    await cover.evaluate((element) => element.dispatchEvent(new Event('error')))

    await expect(cover).toHaveAttribute('data-fallback', 'true')
    await expect(cover).toHaveAttribute('src', /^data:image\/svg\+xml/)
  })

  test('输出规范 URL、社交元数据、结构化数据与爬虫文件', async ({ page, request }) => {
    await openHomepage(page)

    await expect(page.locator('link[rel="canonical"]')).toHaveAttribute('href', 'https://yujian.me')
    await expect(page.locator('meta[property="og:title"]')).toHaveAttribute('content', '遇健我 · 王子健')
    const structuredData = await page.locator('script[type="application/ld+json"]').allTextContents()
    const types = structuredData.flatMap(value => {
      const parsed = JSON.parse(value) as { '@graph'?: Array<{ '@type'?: string }> }
      return parsed['@graph']?.map(item => item['@type']) ?? []
    })
    expect(types).toEqual(expect.arrayContaining([
      'MusicGroup',
      'MusicRecording',
      'VideoObject',
      'MusicEvent',
    ]))

    const robots = await request.get('/robots.txt')
    expect(robots.ok()).toBe(true)
    expect(await robots.text()).toContain('Sitemap: https://yujian.me/sitemap.xml')

    const sitemap = await request.get('/sitemap.xml')
    expect(sitemap.ok()).toBe(true)
    expect(await sitemap.text()).toContain('<loc>https://yujian.me/</loc>')
  })
})

test.describe('移动首页', () => {
  test.use({ viewport: { width: 390, height: 844 } })

  test('所有操作无需悬浮且触控目标稳定', async ({ page }, testInfo) => {
    await openHomepage(page)

    await expect(page.locator('.music-cover-state').first()).toHaveCSS('opacity', '1')
    const undersizedTargets = await page.locator('a, button, summary').evaluateAll((elements) => (
      elements.flatMap((element) => {
        const rect = element.getBoundingClientRect()
        const style = getComputedStyle(element)
        if (
          rect.width === 0
          || rect.height === 0
          || style.display === 'none'
          || style.visibility === 'hidden'
        ) {
          return []
        }
        if (rect.width >= 44 && rect.height >= 44) {
          return []
        }
        return [{
          name: element.getAttribute('aria-label') || element.textContent?.trim() || element.tagName,
          width: rect.width,
          height: rect.height,
        }]
      })
    ))
    expect(undersizedTargets).toEqual([])
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBeLessThanOrEqual(1)
    await expectHeroPixels(page)
    await page.screenshot({ path: testInfo.outputPath('home-mobile.png'), fullPage: true })
  })

  test('试听 Dock 不遮挡页脚操作', async ({ page }) => {
    await openHomepage(page)
    await page.getByTestId('preview-trigger').first().click()
    await expect(page.getByTestId('audio-dock')).toBeVisible()
    await page.locator('footer').scrollIntoViewIfNeeded()

    const dockBox = await page.getByTestId('audio-dock').boundingBox()
    const footerTargets = await page.locator('footer a, footer button, footer summary').evaluateAll((elements) => (
      elements.flatMap((element) => {
        const rect = element.getBoundingClientRect()
        return rect.width && rect.height
          ? [{ name: element.getAttribute('aria-label') || element.textContent?.trim(), bottom: rect.bottom }]
          : []
      })
    ))
    expect(dockBox).not.toBeNull()
    expect(footerTargets.every(target => target.bottom <= dockBox!.y)).toBe(true)
  })

  test('现场卡片在窄屏上统一使用三列布局', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/events/')
    await waitForLocaleHydration(page)

    const cards = page.locator('[data-testid="event-card"] a')
    await expect(cards).toHaveCount(2)
    await cards.first().evaluate((element) => {
      element.classList.add('event-row--with-poster')
      const poster = document.createElement('img')
      poster.className = 'event-poster'
      poster.alt = 'Test poster'
      element.prepend(poster)
    })
    await expect(cards.first()).toHaveClass(/event-row--with-poster/)
    for (const card of await cards.all()) {
      const columns = await card.evaluate(element => getComputedStyle(element).gridTemplateColumns.split(' ').length)
      expect(columns).toBe(3)
    }
  })

  test('影像卡片保留单个平台直达并让五个平台菜单越过卡片边界', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto('/video/')
    await waitForLocaleHydration(page)

    const card = page.locator('.video-card').first()
    const primary = card.getByTestId('platform-primary')
    await expect(primary).toHaveAttribute('href', 'https://space.bilibili.com/21096618')
    const popupPromise = page.waitForEvent('popup')
    await primary.click()
    const popup = await popupPromise
    await expect(popup).toHaveURL('https://space.bilibili.com/21096618')
    await popup.close()

    const menuBounds = await card.evaluate((element) => {
      const links = element.querySelector('.platform-links')!
      const mobile = links.querySelector('.platform-links__mobile')!
      const scopes = links.getAttributeNames().filter(name => name.startsWith('data-v-'))
      const details = document.createElement('details')
      details.className = 'platform-more'
      details.open = true
      const menu = document.createElement('div')
      menu.className = 'platform-more__menu'
      for (let index = 0; index < 5; index++) {
        const link = document.createElement('a')
        link.href = `https://example.com/platform-${index + 1}`
        link.target = '_blank'
        link.rel = 'noopener noreferrer'
        link.textContent = `平台 ${index + 1}`
        for (const scope of scopes) link.setAttribute(scope, '')
        menu.append(link)
      }
      const summary = document.createElement('summary')
      summary.textContent = '更多平台'
      for (const scope of scopes) {
        details.setAttribute(scope, '')
        menu.setAttribute(scope, '')
        summary.setAttribute(scope, '')
      }
      details.append(summary, menu)
      mobile.append(details)
      summary.focus()
      const cardBounds = element.getBoundingClientRect()
      const bounds = menu.getBoundingClientRect()
      return {
        cardOverflow: getComputedStyle(element).overflow,
        posterOverflow: element.querySelector('.video-poster-frame')
          ? getComputedStyle(element.querySelector('.video-poster-frame')!).overflow
          : 'missing',
        menuLinkCount: menu.querySelectorAll('a').length,
        menuTop: bounds.top,
        cardTop: cardBounds.top,
      }
    })

    expect(menuBounds.cardOverflow).toBe('visible')
    expect(menuBounds.posterOverflow).toBe('hidden')
    expect(menuBounds.menuLinkCount).toBe(5)
    expect(menuBounds.menuTop).toBeLessThan(menuBounds.cardTop)
    await page.context().route('https://example.com/platform-5', route => route.fulfill({ status: 200, body: 'opened' }))
    const menuPopupPromise = page.context().waitForEvent('page')
    await page.locator('.video-card').first().locator('.platform-more__menu a').last().click()
    const menuPopup = await menuPopupPromise
    await expect(menuPopup).toHaveURL('https://example.com/platform-5')
    await menuPopup.close()
  })
})

test.describe('语言偏好', () => {
  test.use({ locale: 'en-US', viewport: { width: 390, height: 844 } })

  test('使用标准浏览器语言、非阻断提示和 localStorage，且不改变 URL', async ({ page }) => {
    await openHomepage(page)

    expect(await page.evaluate(() => navigator.languages)).toContain('en-US')
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    await expect(page.getByRole('status')).toContainText('Switched to English')
    expect(await page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('en')
    expect(new URL(page.url()).pathname).toBe('/')

    await page.getByRole('button', { name: '切换为中文' }).click()
    await expect(page.locator('html')).toHaveAttribute('lang', 'zh-CN')
    expect(await page.evaluate(() => localStorage.getItem('yujian:locale'))).toBe('zh-CN')
    expect(new URL(page.url()).pathname).toBe('/')
  })
})
