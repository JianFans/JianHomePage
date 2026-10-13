import { mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import EventWorkbench from '../../components/EventWorkbench.vue'
import { useEventWorkspace } from '../../composables/useEventWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

/** 挂载真实状态机和表单，使用完整 canonical 应用事务。 */
function mountEvent() {
  const source = structuredClone(fixture)
  source.assets.push({ ...source.assets[0]!, id: 'asset_gif_test', kind: 'gif', mimeType: 'image/gif', src: '/media/test.gif' })
  const editorText = ref(JSON.stringify(source))
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const busy = ref(false)
  const locale = ref<'zh-CN' | 'en'>('zh-CN')
  const event = reactive(useEventWorkspace({ editorText, busy }))
  const host = defineComponent({
    components: { EventWorkbench }, setup: () => ({ event, snapshot, busy, locale }),
    template: `<EventWorkbench v-model:draft="event.draft" :snapshot="snapshot" :locale="locale" :busy="busy"
      :dirty="event.dirty" :stale="event.stale" :pending="Boolean(event.pending)" :error="event.error" :issues="event.issues"
      @open="event.open" @apply="event.apply" @close="event.close" @keep="event.keepEditing" @discard="event.discardPending" />`,
  })
  return { wrapper: mount(host), event, editorText, busy, locale }
}

describe('现场工作台组件', () => {
  it('默认首条与只读 ID，切换新增记录保留全部输入及原生编辑区隔离', async () => {
    const { wrapper, event } = mountEvent()
    expect(wrapper.find('[data-testid="event-fields"]').exists()).toBe(false)
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    expect(wrapper.get('[data-testid="event-id"]').text()).toBe(fixture.events[0]!.id)
    expect(wrapper.find('input[data-testid="event-id"]').exists()).toBe(false)
    const firstInput = wrapper.get('[data-testid="event-title-zh"]').element
    await wrapper.get('[data-testid="event-title-zh"]').setValue('第一条保留')
    await wrapper.get('[data-testid="event-add"]').trigger('click')
    const id = event.draft!.events.at(-1)!.id
    expect(wrapper.get('[data-testid="event-title-zh"]').element).not.toBe(firstInput)
    await wrapper.get('[data-testid="event-title-zh"]').setValue('新记录保留')
    await wrapper.get(`[data-testid="event-record-${fixture.events[0]!.id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="event-title-zh"]').element).toHaveProperty('value', '第一条保留')
    await wrapper.get(`[data-testid="event-record-${id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="event-title-zh"]').element).toHaveProperty('value', '新记录保留')
    expect(wrapper.find('[data-testid="event-discard-prompt"]').exists()).toBe(false)
  })
  it('绑定双语字段、日期原文、状态和 HTTPS 链接，清空所有选填属性', async () => {
    const { wrapper, event, locale } = mountEvent()
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    for (const name of ['title', 'city', 'venue']) await wrapper.get(`[data-testid="event-${name}-en"]`).setValue('English')
    await wrapper.get('[data-testid="event-datetime"]').setValue('2030-01-02T20:00:00+08:00')
    await wrapper.get('[data-testid="event-status"]').setValue('sold-out')
    await wrapper.get('[data-testid="event-detail-url"]').setValue('https://example.com/event')
    await wrapper.get('[data-testid="event-poster"]').setValue('asset_gif_test')
    expect(event.draft!.events[0]).toMatchObject({ title: { en: 'English' }, city: { en: 'English' }, venue: { en: 'English' }, dateTime: '2030-01-02T20:00:00+08:00', status: 'sold-out', detailUrl: 'https://example.com/event', posterAssetId: 'asset_gif_test' })
    for (const name of ['title', 'city', 'venue']) await wrapper.get(`[data-testid="event-${name}-en"]`).setValue('')
    await wrapper.get('[data-testid="event-poster"]').setValue('')
    expect(event.draft!.events[0]).not.toHaveProperty('posterAssetId')
    for (const value of [event.draft!.events[0]!.title, event.draft!.events[0]!.city, event.draft!.events[0]!.venue]) expect(value).not.toHaveProperty('en')
    expect(wrapper.get('[data-testid="event-datetime"]').attributes()).toMatchObject({ type: 'text', spellcheck: 'false', 'aria-describedby': 'event-timezone-hint' })
    expect(wrapper.get('#event-timezone-hint').text()).toContain('±HH:MM')
    expect(wrapper.get('[data-testid="event-detail-url"]').attributes('type')).toBe('url')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-add"]').attributes('aria-label')).toBe('Add event')
    expect(wrapper.get('[data-testid="event-status"]').text()).toContain('Sold out')
    expect(wrapper.get('[data-testid="event-title-en"]').element).toHaveProperty('value', '')
  })
  it('海报仅图片/GIF，非法 JSON 保留最近有效选项与输入，确认取消保持草稿', async () => {
    const { wrapper, editorText } = mountEvent()
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    const ids = wrapper.get('[data-testid="event-poster"]').findAll('option').map(option => option.attributes('value'))
    for (const asset of fixture.assets) expect(ids.includes(asset.id)).toBe(['image', 'gif'].includes(asset.kind))
    expect(ids).toContain('asset_gif_test')
    expect(wrapper.find('img, video, audio').exists()).toBe(false)
    await wrapper.get('[data-testid="event-title-zh"]').setValue('保留')
    editorText.value = '{'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-title-zh"]').element).toHaveProperty('value', '保留')
    expect(wrapper.get('[data-testid="event-poster"]').findAll('option').map(option => option.attributes('value'))).toEqual(ids)
    expect(wrapper.get('[data-testid="event-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="event-close"]').trigger('click')
    await wrapper.get('[data-testid="event-keep"]').trigger('click')
    expect(wrapper.find('[data-testid="event-fields"]').exists()).toBe(true)
    await wrapper.get('[data-testid="event-close"]').trigger('click')
    await wrapper.get('[data-testid="event-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="event-fields"]').exists()).toBe(false)
  })
  it('诊断最多八条并本地化剩余数量，busy 锁定 fieldset 与全部按钮', async () => {
    const { wrapper, event, busy, locale } = mountEvent()
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    event.draft!.events[0]!.posterAssetId = 'asset_preview_sample'
    event.apply()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-error"]').text()).toContain('/events/0/posterAssetId')
    expect(wrapper.get('[data-testid="event-error"]').text()).toContain('asset-kind')
    event.issues = Array.from({ length: 11 }, (_, index) => ({ path: `/events/${index}/title`, source: 'schema' as const, code: 'required' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-error"]').findAll('li')).toHaveLength(8)
    expect(wrapper.get('[data-testid="event-error"]').text()).toContain('另有 3 项错误')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-error"]').text()).toContain('3 more issues')
    await wrapper.get('[data-testid="event-open"]').trigger('click')
    busy.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="event-fields"]').attributes('disabled')).toBeDefined()
    for (const button of wrapper.findAll('button')) expect(button.attributes('disabled') !== undefined || Boolean(button.element.closest('fieldset[disabled]'))).toBe(true)
  })
})
