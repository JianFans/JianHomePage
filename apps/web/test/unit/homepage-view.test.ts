import type { YujianContentSnapshot } from '@yujian/schema'
import { flushPromises, mount } from '@vue/test-utils'
import fixture from '../../../../content/fixtures/homepage.json'
import HomepageView from '../../components/home/HomepageView.vue'
import { createAudioPlayerController } from '../../utils/audio-player'

/** Keep each render independent so preview edits cannot leak between mounts. */
function snapshotFixture(): YujianContentSnapshot {
  return structuredClone(fixture) as unknown as YujianContentSnapshot
}

/** Exercise the real controller while replacing only browser media playback. */
function playerFixture() {
  return createAudioPlayerController({
    createAudio: () => ({
      src: '',
      preload: '',
      currentTime: 0,
      duration: 30,
      paused: true,
      play: async () => {},
      pause: () => {},
      load: () => {},
      addEventListener: () => {},
      removeEventListener: () => {},
    }),
  })
}

/** Use a video slide to verify preview permission without changing content policy. */
function videoSnapshot(autoplay = true) {
  const snapshot = snapshotFixture()
  const slide = snapshot.heroSlides[0]!
  const asset = snapshot.assets.find(item => item.id === slide.assetId)!
  snapshot.assets.push({ ...asset, id: 'preview-video', kind: 'video', src: '/media/hero.mp4', mimeType: 'video/mp4' })
  snapshot.heroSlides = [{ ...slide, mediaKind: 'video', assetId: 'preview-video', autoplay }]
  return snapshot
}

it('retains footer platforms when the artist has no platform entries', () => {
  const snapshot = snapshotFixture()
  snapshot.artist.platformLinks = []
  const wrapper = mount(HomepageView, { props: { snapshot, locale: 'en', player: playerFixture() } })
  expect(wrapper.find('footer .platform-links').exists()).toBe(true)
  wrapper.unmount()
})

beforeEach(() => {
  vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({
    matches: false,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
  }))
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('HomepageView explicit rendering boundary', () => {
  it('音乐人实际展示时不重复页脚平台，停用或引用缺失时页脚保留一次入口', async () => {
    const snapshot = snapshotFixture()
    const wrapper = mount(HomepageView, { props: { snapshot, locale: 'en', player: playerFixture() } })
    expect(wrapper.find('footer .platform-links').exists()).toBe(false)
    const section = snapshot.homepage.sections.find(section => section.type === 'artist')!
    section.enabled = false
    await wrapper.setProps({ snapshot: structuredClone(snapshot) })
    expect(wrapper.find('footer .platform-links').exists()).toBe(true)
    section.enabled = true
    // @ts-expect-error Deliberately supply a missing reference to verify defensive rendering.
    section.itemIds = ['missing_artist']
    await wrapper.setProps({ snapshot: structuredClone(snapshot) })
    expect(wrapper.find('footer .platform-links').exists()).toBe(true)
    expect(wrapper.find('footer a[href="#top"]').exists()).toBe(false)
    wrapper.unmount()
  })
  it('renders all six sections and site identity from the supplied snapshot', () => {
    const snapshot = snapshotFixture()
    snapshot.site.brand.en = 'Draft brand'
    const wrapper = mount(HomepageView, { props: { snapshot, locale: 'en', player: playerFixture() } })

    expect(wrapper.get('h1').text()).toBe('Draft brand')
    expect(wrapper.get('header').text()).toContain('Draft brand')
    expect(wrapper.get('footer').text()).toContain('Draft brand')
    expect(wrapper.findAll('[data-home-section]').map(section => section.attributes('data-home-section')))
      .toEqual(['hero', 'music', 'video', 'event', 'moment', 'artist'])
    expect(wrapper.findAll('[data-testid="music-card"]')).toHaveLength(5)
    expect(wrapper.get('footer').text()).toContain('© 2026')
  })

  it('emits locale intent and reacts to the locale supplied by its host', async () => {
    const wrapper = mount(HomepageView, {
      props: { snapshot: snapshotFixture(), locale: 'zh-CN', player: playerFixture() },
    })

    await wrapper.get('[aria-label="Switch to English"]').trigger('click')
    expect(wrapper.emitted('toggleLocale')).toEqual([[]])
    expect(wrapper.get('h1').text()).toBe('遇健我')
    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('h1').text()).toBe('Meet Jian')
  })

  it('routes music preview and active playback state through the supplied controller', async () => {
    const player = playerFixture()
    const otherPlayer = playerFixture()
    const wrapper = mount(HomepageView, {
      props: { snapshot: snapshotFixture(), locale: 'en', player },
    })
    const button = wrapper.findAll('[data-testid="preview-trigger"]')[0]!

    await button.trigger('click')
    await flushPromises()
    expect(player.current.value).not.toBeNull()
    expect(player.status.value).toBe('playing')
    expect(player.queue.value.length).toBeGreaterThan(1)
    expect(button.attributes('aria-pressed')).toBe('true')
    expect(otherPlayer.current.value).toBeNull()
    await button.trigger('click')
    expect(player.status.value).toBe('paused')
    player.close()
  })

  it('uses snapshot time for hero windows and upcoming events and reacts to replacement', async () => {
    const snapshot = snapshotFixture()
    snapshot.generatedAt = '2030-01-01T00:00:00Z'
    snapshot.heroSlides[0]!.startsAt = '2029-12-01T00:00:00Z'
    snapshot.heroSlides[0]!.endsAt = '2030-02-01T00:00:00Z'
    snapshot.heroSlides[1]!.startsAt = '2031-01-01T00:00:00Z'
    snapshot.heroSlides[2]!.endsAt = '2029-01-01T00:00:00Z'
    const wrapper = mount(HomepageView, { props: { snapshot, locale: 'en', player: playerFixture() } })

    expect(wrapper.findAll('[data-content-anchor]').map(anchor => anchor.attributes('id')))
      .toEqual([snapshot.heroSlides[0]!.id])
    expect(wrapper.find('[data-home-section="event"]').exists()).toBe(false)
    expect(wrapper.get('footer').text()).toContain('© 2030')
    await wrapper.setProps({ snapshot: snapshotFixture() })
    expect(wrapper.find('[data-home-section="event"]').exists()).toBe(true)
    expect(wrapper.get('footer').text()).toContain('© 2026')
  })

  it('preserves duplicate section IDs and follows supplied order and enabled state', () => {
    const snapshot = snapshotFixture()
    const music = snapshot.homepage.sections.find(section => section.type === 'music')!
    snapshot.homepage.sections = [music, { ...music }, { ...snapshot.homepage.sections[0]!, enabled: false }]
    const wrapper = mount(HomepageView, { props: { snapshot, locale: 'en', player: playerFixture() } })

    expect(wrapper.findAll('[data-home-section]').map(section => section.attributes('data-home-section')))
      .toEqual(['music', 'music'])
  })

  it.each([
    [undefined, true, true],
    [false, true, false],
    [true, false, false],
  ])('autoplay permission %s with slide policy %s produces autoplay %s', (permission, policy, expected) => {
    const wrapper = mount(HomepageView, {
      props: { snapshot: videoSnapshot(policy), locale: 'en', player: playerFixture(), autoplayEnabled: permission },
    })

    expect(wrapper.get('video').element.autoplay).toBe(expected)
    expect(wrapper.find('[aria-label="Turn on sound"]').exists()).toBe(true)
    if (!expected) expect(wrapper.find('[aria-label="Play video"]').exists()).toBe(true)
  })

  it('keeps manual video playback available when autoplay is disabled', async () => {
    const wrapper = mount(HomepageView, {
      props: { snapshot: videoSnapshot(), locale: 'en', player: playerFixture(), autoplayEnabled: false },
    })
    const media = wrapper.get('video').element
    const play = vi.spyOn(media, 'play').mockImplementation(async () => {
      Object.defineProperty(media, 'paused', { value: false, configurable: true })
      await wrapper.get('video').trigger('play')
    })

    await wrapper.get('[aria-label="Play video"]').trigger('click')
    await flushPromises()
    expect(play).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it.each([false, true])('stops the previous shared-src video and applies autoplay permission %s to the next one', async (autoplayEnabled) => {
    const snapshot = videoSnapshot()
    snapshot.heroSlides.push({ ...snapshot.heroSlides[0]!, id: 'second-video' })
    snapshot.homepage.sections.find(section => section.type === 'hero')!.itemIds.push('second-video')
    const wrapper = mount(HomepageView, {
      props: { snapshot, locale: 'en', player: playerFixture(), autoplayEnabled },
    })
    const media = wrapper.get('video').element
    const source = media.src
    const load = vi.spyOn(media, 'load')
    Object.defineProperty(media, 'paused', { value: false, configurable: true })
    const pause = vi.spyOn(media, 'pause').mockImplementation(() => {
      Object.defineProperty(media, 'paused', { value: true, configurable: true })
    })

    await wrapper.get('[aria-label="Next slide"]').trigger('click')

    expect(pause).toHaveBeenCalledOnce()
    expect(media.paused).toBe(true)
    expect(media.getAttribute('src')).toBeNull()
    expect(load).toHaveBeenCalledOnce()
    const nextMedia = wrapper.get('video').element
    expect(nextMedia.src).toBe(source)
    expect(nextMedia.autoplay).toBe(autoplayEnabled)
    if (autoplayEnabled) {
      // A fresh element lets the browser apply autoplay even when src is unchanged.
      expect(nextMedia).not.toBe(media)
      expect(wrapper.find('[aria-label="Pause video"]').exists()).toBe(true)
      media.dispatchEvent(new Event('pause'))
      media.dispatchEvent(new Event('error'))
      await flushPromises()
      expect(wrapper.find('[aria-label="Pause video"]').exists()).toBe(true)
      expect(wrapper.find('video').exists()).toBe(true)
    } else {
      expect(nextMedia.paused).toBe(true)
      expect(wrapper.find('[aria-label="Play video"]').exists()).toBe(true)
    }
    wrapper.unmount()
  })

  it.each([
    ['unmount', 'resolve'],
    ['unmount', 'reject'],
    ['replace', 'resolve'],
    ['replace', 'reject'],
    ['switch', 'resolve'],
    ['switch', 'reject'],
  ])('ignores pending manual playback after %s followed by %s', async (action, outcome) => {
    const snapshot = videoSnapshot()
    const slide = snapshot.heroSlides[0]!
    snapshot.heroSlides.push({ ...slide, id: 'second-video' })
    const hero = snapshot.homepage.sections.find(section => section.type === 'hero')!
    hero.itemIds.push('second-video')
    const errors: unknown[] = []
    const wrapper = mount(HomepageView, {
      props: { snapshot, locale: 'en', player: playerFixture(), autoplayEnabled: false },
      global: { config: { errorHandler: error => errors.push(error) } },
    })
    const media = wrapper.get('video').element
    let settle!: () => void
    const pending = new Promise<void>((resolve, reject) => {
      settle = outcome === 'resolve' ? resolve : () => reject(new Error('playback interrupted'))
    })
    vi.spyOn(media, 'play').mockReturnValue(pending)

    await wrapper.get('[aria-label="Play video"]').trigger('click')
    if (action === 'unmount') wrapper.unmount()
    else if (action === 'replace') await wrapper.setProps({ snapshot: snapshotFixture() })
    else await wrapper.get('[aria-label="Next slide"]').trigger('click')
    Object.defineProperty(media, 'paused', { value: false, configurable: true })
    settle()
    await flushPromises()

    expect(errors).toEqual([])
    if (action === 'switch') expect(wrapper.find('[aria-label="Play video"]').exists()).toBe(true)
    if (action !== 'unmount') wrapper.unmount()
  })

  it('keeps a rejected manual playback request paused and retryable', async () => {
    const errors: unknown[] = []
    const wrapper = mount(HomepageView, {
      props: { snapshot: videoSnapshot(), locale: 'en', player: playerFixture(), autoplayEnabled: false },
      global: { config: { errorHandler: error => errors.push(error) } },
    })
    vi.spyOn(wrapper.get('video').element, 'play').mockRejectedValue(new Error('browser denied playback'))

    await wrapper.get('[aria-label="Play video"]').trigger('click')
    await flushPromises()

    expect(errors).toEqual([])
    expect(wrapper.find('[aria-label="Play video"]').exists()).toBe(true)
    expect(wrapper.find('video').exists()).toBe(true)
    wrapper.unmount()
  })

  it.each([
    ['roundtrip', 'resolve'],
    ['roundtrip', 'reject'],
    ['repeat', 'resolve'],
    ['repeat', 'reject'],
  ])('keeps newer playback after %s when the old request ends with %s', async (action, outcome) => {
    const snapshot = videoSnapshot()
    snapshot.heroSlides.push({ ...snapshot.heroSlides[0]!, id: 'second-video' })
    snapshot.homepage.sections.find(section => section.type === 'hero')!.itemIds.push('second-video')
    const wrapper = mount(HomepageView, {
      props: { snapshot, locale: 'en', player: playerFixture(), autoplayEnabled: false },
    })
    const media = wrapper.get('video').element
    let settle!: () => void
    const pending = new Promise<void>((resolve, reject) => {
      settle = outcome === 'resolve' ? resolve : () => reject(new Error('superseded playback'))
    })
    vi.spyOn(media, 'play').mockReturnValueOnce(pending)

    await wrapper.get('[aria-label="Play video"]').trigger('click')
    if (action === 'roundtrip') {
      await wrapper.get('[aria-label="Next slide"]').trigger('click')
      await wrapper.get('[aria-label="Previous slide"]').trigger('click')
    }
    const latestMedia = wrapper.get('video').element
    const succeed = async () => {
      Object.defineProperty(latestMedia, 'paused', { value: false, configurable: true })
      latestMedia.dispatchEvent(new Event('play'))
    }
    if (latestMedia === media) vi.mocked(media.play).mockImplementationOnce(succeed)
    else vi.spyOn(latestMedia, 'play').mockImplementationOnce(succeed)
    await wrapper.get('[aria-label="Play video"]').trigger('click')
    await flushPromises()
    expect(wrapper.find('[aria-label="Pause video"]').exists()).toBe(true)

    settle()
    await flushPromises()

    expect(wrapper.find('[aria-label="Pause video"]').exists()).toBe(true)
    wrapper.unmount()
  })

  it('invalidates pending playback on a roundtrip even without a newer manual request', async () => {
    const snapshot = videoSnapshot()
    snapshot.heroSlides.push({ ...snapshot.heroSlides[0]!, id: 'second-video' })
    snapshot.homepage.sections.find(section => section.type === 'hero')!.itemIds.push('second-video')
    const wrapper = mount(HomepageView, {
      props: { snapshot, locale: 'en', player: playerFixture(), autoplayEnabled: false },
    })
    const media = wrapper.get('video').element
    let resolve!: () => void
    vi.spyOn(media, 'play').mockReturnValue(new Promise<void>((done) => { resolve = done }))

    await wrapper.get('[aria-label="Play video"]').trigger('click')
    await wrapper.get('[aria-label="Next slide"]').trigger('click')
    await wrapper.get('[aria-label="Previous slide"]').trigger('click')
    Object.defineProperty(media, 'paused', { value: false, configurable: true })
    resolve()
    await flushPromises()

    expect(wrapper.find('[aria-label="Play video"]').exists()).toBe(true)
    wrapper.unmount()
  })
})
