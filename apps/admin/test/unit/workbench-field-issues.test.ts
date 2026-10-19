import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import EventWorkbench from '../../components/EventWorkbench.vue'
import MusicWorkbench from '../../components/MusicWorkbench.vue'
import HomepageWorkbench from '../../components/HomepageWorkbench.vue'
import VideoWorkbench from '../../components/VideoWorkbench.vue'
import MomentWorkbench from '../../components/MomentWorkbench.vue'
import { useEventWorkspace } from '../../composables/useEventWorkspace'
import { useMusicWorkspace } from '../../composables/useMusicWorkspace'
import { useHomepageWorkspace } from '../../composables/useHomepageWorkspace'
import { useVideoWorkspace } from '../../composables/useVideoWorkspace'
import { useMomentWorkspace } from '../../composables/useMomentWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'
import { diagnosticLabel, focusSnapshotIssue } from '../../utils/issue-accessibility'
import type { MusicDraft } from '../../utils/music-workbench'

const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount() })

/** Exercise canonical validation and focus in an attached real workbench. */
function mountWorkbench(kind: 'event' | 'music' | 'homepage' | 'video' | 'moment', locale: 'zh-CN' | 'en' = 'zh-CN') {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const form = reactive(kind === 'event' ? useEventWorkspace({ editorText, busy })
    : kind === 'homepage' ? useHomepageWorkspace({ editorText, busy })
      : kind === 'video' ? useVideoWorkspace({ editorText, busy })
        : kind === 'moment' ? useMomentWorkspace({ editorText, busy })
          : useMusicWorkspace({ editorText, busy, snapshot }))
  const host = defineComponent({
    components: { Workbench: { event: EventWorkbench, music: MusicWorkbench, homepage: HomepageWorkbench, video: VideoWorkbench, moment: MomentWorkbench }[kind] },
    setup: () => ({ form, snapshot, busy, locale }),
    template: `<Workbench v-model:draft="form.draft" :snapshot="snapshot" :locale="locale" :busy="busy"
      :dirty="form.dirty" :stale="form.stale" :error="form.error" :issues="form.issues" :pending="false" studio
      @select="form.select" @open="form.open" @apply="form.apply" />`,
  })
  const wrapper = mount(host, { attachTo: document.body })
  wrappers.push(wrapper)
  return { wrapper, form }
}

/** Resolve every description token so tests cover assistive text, not only attributes. */
function description(element: Element) {
  return (element.getAttribute('aria-describedby') || '').split(' ').map(id => document.getElementById(id)?.textContent || '').join(' ')
}

describe('canonical field issue navigation', () => {
  it.each([
    { kind: 'music', selector: '[data-testid="music-fields"] > details [data-testid="link-url"]', path: '/releases/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
    { kind: 'music', selector: '[data-testid="track-editor"] [data-testid="link-url"]', path: '/tracks/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
    { kind: 'video', selector: '[data-testid="link-url"]', path: '/videos/0/platformLinks/0/url', invalid: 'http://example.com', valid: 'https://example.com' },
    { kind: 'music', selector: '[data-testid="music-date"]', path: '/releases/0/releaseDate', invalid: '', valid: '2026-10-15' },
    { kind: 'music', selector: '[data-testid="track-duration"]', path: '/tracks/0/durationSeconds', invalid: '0', valid: '180' },
    { kind: 'music', selector: '[data-testid="track-preview-duration"]', path: '/tracks/0/previewDurationSeconds', invalid: '91', valid: '30' },
    { kind: 'music', selector: '[data-testid="credit-name"]', path: '/tracks/0/credits/0/name', invalid: '', valid: '王子健' },
    { kind: 'event', selector: '[data-testid="event-city-zh"]', path: '/events/0/city/zh-CN', invalid: '', valid: '杭州' },
    { kind: 'homepage', selector: '[data-testid="homepage-starts-at"]', path: '/heroSlides/0/startsAt', invalid: '2030-01-01T20:00:00', valid: '' },
    { kind: 'homepage', selector: '[data-testid="homepage-ends-at"]', path: '/heroSlides/0/endsAt', invalid: 'invalid', valid: '' },
    { kind: 'homepage', selector: '[data-testid="homepage-focal-x"]', path: '/heroSlides/0/focalPoint/x', invalid: '101', valid: '50' },
    { kind: 'homepage', selector: '[data-testid="homepage-section-section_music"] [data-testid="homepage-more-url"]', path: '/homepage/sections/1/moreLink/url', invalid: 'http://example.com', valid: 'https://example.com' },
    { kind: 'homepage', selector: '[data-testid="homepage-section-section_music"] [data-testid="homepage-section-limit"]', path: '/homepage/sections/1/limit', invalid: '0', valid: '5' },
  ] as const)('locates $path with localized feedback and clears it after correction', async ({ kind, selector, path, invalid, valid }) => {
    const { wrapper, form } = mountWorkbench(kind, 'en')
    if (kind === 'music') await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    else await wrapper.get(`[data-testid="${kind}-open"]`).trigger('click')
    await wrapper.get(selector).setValue(invalid)
    if (kind === 'event') await wrapper.get(`[data-testid="event-record-${fixture.events[1]!.id}"]`).trigger('click')
    expect(form.apply()).toBe(false)
    await flushPromises()
    expect(form.issues[0]?.path).toBe(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    const field = wrapper.get(selector)
    expect(document.activeElement).toBe(field.element)
    expect(field.attributes('data-snapshot-path')).toBe(path)
    expect(field.attributes('aria-invalid')).toBe('true')
    expect(description(field.element)).toContain(path)
    expect(description(field.element)).toContain(diagnosticLabel(form.issues[0]!.code, 'en'))
    for (let parent = field.element.parentElement; parent; parent = parent.parentElement) {
      if (parent instanceof HTMLDetailsElement) expect(parent.open).toBe(true)
    }
    await field.setValue(valid)
    expect(form.apply()).toBe(true)
    await flushPromises()
    expect(field.attributes('aria-invalid')).toBeUndefined()
    expect(description(field.element)).not.toContain(path)
  })

  it('locates an empty custom platform label in a collapsed link group', async () => {
    const { wrapper, form } = mountWorkbench('music')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    const link = wrapper.get('[data-testid="platform-link"]')
    await link.get('input[type="checkbox"]').setValue(true)
    expect(form.apply()).toBe(false)
    await flushPromises()
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    const input = link.get('input[type="text"]')
    expect(document.activeElement).toBe(input.element)
    expect(description(input.element)).toContain('/releases/0/platformLinks/0/label/zh-CN')
  })

  it('expands all collapsed ancestors before focusing a nested field', async () => {
    const { wrapper } = mountWorkbench('music')
    const outer = document.createElement('details')
    const inner = document.createElement('details')
    const input = document.createElement('input')
    input.dataset.snapshotPath = '/tracks/0/platformLinks/0/label/en'
    inner.append(input)
    outer.append(inner)
    wrapper.element.append(outer)
    expect(await focusSnapshotIssue(wrapper.element, [{ path: input.dataset.snapshotPath, source: 'schema', code: 'minLength' }])).toBe(true)
    expect(outer.open).toBe(true)
    expect(inner.open).toBe(true)
    expect(document.activeElement).toBe(input)
  })

  it('locates the add-link control when every required video link was removed', async () => {
    const { wrapper, form } = mountWorkbench('video')
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    while (wrapper.find('[data-testid="platform-link"]').exists()) {
      await wrapper.get('[data-testid="platform-link"] button').trigger('click')
    }
    expect(form.apply()).toBe(false)
    await flushPromises()
    expect(form.issues[0]?.path).toBe('/videos/0/platformLinks')
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    const add = wrapper.get('[data-testid="link-add"]')
    expect(document.activeElement).toBe(add.element)
    expect(description(add.element)).toContain('项目过少')
  })

  it('allows removing all hero records and references without a required-item error', async () => {
    const { wrapper, form } = mountWorkbench('homepage')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    for (const hero of fixture.heroSlides) {
      await wrapper.get(`[data-testid="homepage-hero-${hero.id}"]`).trigger('click')
      await wrapper.get('[data-testid="homepage-hero-remove"]').trigger('click')
    }
    expect(form.apply()).toBe(true)
    await flushPromises()
    expect(form.issues).toEqual([])
    const select = wrapper.get('[data-testid="homepage-section-section_hero"] [data-testid="homepage-item-add-select"]')
    expect(select.attributes('aria-invalid')).toBeUndefined()
  })

  it.each(['reorder', 'remove', 'duplicate'] as const)('keeps track diagnostics aligned with canonical order after %s', async (operation) => {
    const { wrapper, form } = mountWorkbench('music')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    const original = wrapper.findAll('[data-testid="track-editor"]')[0]!
    await wrapper.get('[data-testid="track-add"]').trigger('click')
    const added = wrapper.findAll('[data-testid="track-editor"]')[1]!
    await added.get('[data-testid="track-title-zh"]').setValue('新增曲目')
    await added.get('[data-testid="track-duration"]').setValue('0')
    if (operation === 'reorder') await added.get('[data-testid="track-up"]').trigger('click')
    if (operation === 'remove') await original.get('[data-testid="track-remove"]').trigger('click')
    if (operation === 'duplicate') {
      // A canonical snapshot may reference the same track more than once.
      const music = form as { draft: MusicDraft | null }
      music.draft!.tracks.unshift(music.draft!.tracks[0]!)
    }
    expect(form.apply()).toBe(false)
    await flushPromises()
    const index = fixture.tracks.length - (operation === 'remove' ? 1 : 0)
    const path = `/tracks/${index}/durationSeconds`
    expect(form.issues[0]?.path).toBe(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    const field = wrapper.get(`[data-snapshot-path="${path}"]`)
    expect(document.activeElement).toBe(field.element)
    expect(description(field.element)).toContain(path)
    await field.setValue('180')
    expect(form.apply()).toBe(true)
  })

  it('locates an invalid homepage external link after selecting another hero', async () => {
    const { wrapper, form } = mountWorkbench('homepage')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('external')
    await wrapper.get('[data-testid="homepage-target-url"]').setValue('http://example.com')
    await wrapper.get(`[data-testid="homepage-hero-${fixture.heroSlides[1]!.id}"]`).trigger('click')
    expect(form.apply()).toBe(false)
    await flushPromises()
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    const field = wrapper.get('[data-testid="homepage-target-url"]')
    expect(document.activeElement).toBe(field.element)
    expect(description(field.element)).toContain('/heroSlides/0/target/link/url')
  })

  it.each(['add', 'remove'] as const)('reveals an invalid non-current hero after %s', async (operation) => {
    const { wrapper, form } = mountWorkbench('homepage')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get(`[data-testid="homepage-hero-${operation}"]`).trigger('click')
    const invalidId = wrapper.get('[data-testid="homepage-hero-id"]').text()
    if (operation === 'add') await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('新宣传位')
    await wrapper.get('[data-testid="homepage-hero-asset"]').setValue('')
    const otherId = operation === 'add' ? fixture.heroSlides[0]!.id : fixture.heroSlides[2]!.id
    await wrapper.get(`[data-testid="homepage-hero-${otherId}"]`).trigger('click')
    expect(form.apply()).toBe(false)
    await flushPromises()
    const path = `/heroSlides/${operation === 'add' ? fixture.heroSlides.length : 0}/assetId`
    expect(form.issues[0]?.path).toBe(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    expect(wrapper.get('[data-testid="homepage-hero-id"]').text()).toBe(invalidId)
    const asset = wrapper.get('[data-testid="homepage-hero-asset"]')
    expect(document.activeElement).toBe(asset.element)
    expect(asset.attributes('aria-invalid')).toBe('true')
    expect(description(asset.element)).toContain(path)
    await asset.setValue('asset_cover_01')
    expect(form.apply()).toBe(true)
    await flushPromises()
    expect(asset.attributes('aria-invalid')).toBeUndefined()
  })

  it.each([
    { kind: 'video', field: 'video-title-zh', path: '/videos/0/title/zh-CN', second: fixture.videos[1]!.id },
    { kind: 'video', field: 'video-poster', path: '/videos/0/posterAssetId', second: fixture.videos[1]!.id },
    { kind: 'moment', field: 'moment-asset', path: '/moments/0/assetId', second: fixture.moments[1]!.id },
    { kind: 'moment', field: 'moment-caption-zh', path: '/moments/0/caption/zh-CN', second: fixture.moments[1]!.id },
  ] as const)('reveals a non-current $kind record and describes $field', async ({ kind, field, path, second }) => {
    const { wrapper, form } = mountWorkbench(kind)
    await wrapper.get(`[data-testid="${kind}-open"]`).trigger('click')
    const original = wrapper.get(`[data-testid="${kind}-id"]`).text()
    await wrapper.get(`[data-testid="${field}"]`).setValue('')
    await wrapper.get(`[data-testid="${kind}-record-${second}"]`).trigger('click')
    expect(form.apply()).toBe(false)
    expect(form.issues[0]?.path).toBe(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    await flushPromises()
    expect(wrapper.get(`[data-testid="${kind}-id"]`).text()).toBe(original)
    const control = wrapper.get(`[data-testid="${field}"]`)
    expect(document.activeElement).toBe(control.element)
    expect(control.attributes('aria-invalid')).toBe('true')
    expect(description(control.element)).toContain(path)
  })
  it('keeps the existing bilingual schema diagnostic labels', () => {
    expect(diagnosticLabel('additionalProperties', 'zh-CN')).toBe('存在不允许的字段')
    expect(diagnosticLabel('minItems', 'en')).toBe('Too few items')
    expect(diagnosticLabel('invalid-json', 'en')).toBe('Invalid JSON')
  })

  it('focuses an empty required cover with a localized contract description', async () => {
    const { wrapper, form } = mountWorkbench('music')
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="music-cover"]').setValue('')
    expect(form.apply()).toBe(false)
    await flushPromises()
    const cover = wrapper.get('[data-testid="music-cover"]')
    expect(cover.attributes('aria-invalid')).toBe('true')
    expect(description(cover.element)).toContain('/releases/0/coverAssetId')
    expect(description(cover.element)).toContain('字段格式错误')
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    expect(document.activeElement).toBe(cover.element)
  })

  it('reveals the failing event record and preserves the timezone hint', async () => {
    const { wrapper, form } = mountWorkbench('event')
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="event-datetime"]').setValue('2030-01-02T20:00:00')
    expect(form.apply()).toBe(false)
    await focusSnapshotIssue(wrapper.element, form.issues)
    await flushPromises()
    const field = wrapper.get('[data-testid="event-datetime"]').element
    expect(wrapper.get('[data-testid="event-id"]').text()).toBe(fixture.events[0]!.id)
    expect(document.activeElement).toBe(field)
    expect(description(field)).toContain('±HH:MM')
    expect(description(field)).toContain('/events/0/dateTime')
    expect(description(field)).toContain('字段格式错误')
  })

  it.each(['zh-CN', 'en'] as const)('locates an invalid event HTTPS URL with a %s description and clears it after correction', async (locale) => {
    const { wrapper, form } = mountWorkbench('event', locale)
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    await wrapper.get('[data-testid="event-detail-url"]').setValue('http://example.com')
    await wrapper.get(`[data-testid="event-record-${fixture.events[1]!.id}"]`).trigger('click')
    expect(form.apply()).toBe(false)
    await flushPromises()
    const path = '/events/0/detailUrl'
    expect(form.issues[0]?.path).toBe(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    expect(wrapper.get('[data-testid="event-id"]').text()).toBe(fixture.events[0]!.id)
    const field = wrapper.get('[data-testid="event-detail-url"]')
    expect(document.activeElement).toBe(field.element)
    expect(field.attributes('data-snapshot-path')).toBe(path)
    expect(field.attributes('aria-invalid')).toBe('true')
    expect(description(field.element)).toContain(path)
    expect(description(field.element)).toContain(diagnosticLabel(form.issues[0]!.code, locale))
    await field.setValue('https://example.com')
    expect(form.apply()).toBe(true)
    await flushPromises()
    expect(field.attributes('aria-invalid')).toBeUndefined()
    expect(field.attributes('aria-describedby')).toBeUndefined()
    expect(field.attributes('data-snapshot-path')).toBe(path)
  })

  it('binds a new release cover issue to the appended record and focuses it', async () => {
    const { wrapper, form } = mountWorkbench('music')
    await wrapper.get('[data-testid="music-new"]').trigger('click')
    await wrapper.get('[data-testid="release-title-zh"]').setValue('新作品')
    await wrapper.get('[data-testid="music-date"]').setValue('2026-10-15')
    await wrapper.get('[data-testid="track-title-zh"]').setValue('新曲目')
    await wrapper.get('[data-testid="track-duration"]').setValue('180')
    expect(form.apply()).toBe(false)
    await flushPromises()
    const path = `/releases/${fixture.releases.length}/coverAssetId`
    expect(form.issues).toEqual([expect.objectContaining({ path })])
    const cover = wrapper.get('[data-testid="music-cover"]')
    expect(cover.attributes('data-snapshot-path')).toBe(path)
    expect(cover.attributes('aria-invalid')).toBe('true')
    expect(description(cover.element)).toContain(path)
    expect(await focusSnapshotIssue(wrapper.element, form.issues)).toBe(true)
    expect(document.activeElement).toBe(cover.element)
    await cover.setValue('asset_cover_01')
    expect(form.apply()).toBe(true)
    await flushPromises()
    expect(cover.attributes('aria-invalid')).toBeUndefined()
    expect(cover.attributes('aria-describedby')).toBeUndefined()
  })

  it('selects another hero for a hidden internal target instead of focusing an unrelated field', async () => {
    const { wrapper, form } = mountWorkbench('homepage')
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('internal')
    await wrapper.get('[data-testid="homepage-target-content"]').setValue(fixture.events[0]!.id)
    const eventSection = wrapper.get('[data-testid="homepage-section-section_event"]')
    await eventSection.get('[data-testid="homepage-section-enabled"]').setValue(false)
    expect(form.apply()).toBe(false)
    await focusSnapshotIssue(wrapper.element, form.issues)
    await flushPromises()
    const target = wrapper.get('[data-testid="homepage-target-content"]').element
    expect(form.issues[0]?.path).toContain('/target/contentId')
    expect(document.activeElement).toBe(target)
    expect(description(target)).toContain('目标未在首页展示')
    expect(description(target)).toContain(form.issues[0]!.path)
  })
})
