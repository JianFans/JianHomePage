import { flushPromises, mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { afterEach, describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import EventWorkbench from '../../components/EventWorkbench.vue'
import MusicWorkbench from '../../components/MusicWorkbench.vue'
import HomepageWorkbench from '../../components/HomepageWorkbench.vue'
import { useEventWorkspace } from '../../composables/useEventWorkspace'
import { useMusicWorkspace } from '../../composables/useMusicWorkspace'
import { useHomepageWorkspace } from '../../composables/useHomepageWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'
import { diagnosticLabel, focusSnapshotIssue } from '../../utils/issue-accessibility'

const wrappers: ReturnType<typeof mount>[] = []
afterEach(() => { for (const wrapper of wrappers.splice(0)) wrapper.unmount() })

/** Exercise canonical validation and focus in an attached real workbench. */
function mountWorkbench(kind: 'event' | 'music' | 'homepage') {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const form = reactive(kind === 'event' ? useEventWorkspace({ editorText, busy })
    : kind === 'homepage' ? useHomepageWorkspace({ editorText, busy })
      : useMusicWorkspace({ editorText, busy, snapshot }))
  const host = defineComponent({
    components: { Workbench: { event: EventWorkbench, music: MusicWorkbench, homepage: HomepageWorkbench }[kind] },
    setup: () => ({ form, snapshot, busy }),
    template: `<Workbench v-model:draft="form.draft" :snapshot="snapshot" locale="zh-CN" :busy="busy"
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
