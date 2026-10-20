import { mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import MomentWorkbench from '../../components/MomentWorkbench.vue'
import { useMomentWorkspace } from '../../composables/useMomentWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

/** 挂载真实状态机和表单，使用完整 canonical 应用事务。 */
function mountMoment() {
  const source = structuredClone(fixture)
  source.assets.push({ ...source.assets[0]!, id: 'asset_a3c8a851-1654-46e3-9bb7-a8640b852c3f', kind: 'gif', mimeType: 'image/gif', src: '/media/test.gif' })
  const editorText = ref(JSON.stringify(source))
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const busy = ref(false)
  const locale = ref<'zh-CN' | 'en'>('zh-CN')
  const moment = reactive(useMomentWorkspace({ editorText, busy }))
  const host = defineComponent({
    components: { MomentWorkbench }, setup: () => ({ moment, snapshot, busy, locale }),
    template: `<MomentWorkbench v-model:draft="moment.draft" :snapshot="snapshot" :locale="locale" :busy="busy"
      :dirty="moment.dirty" :stale="moment.stale" :pending="Boolean(moment.pending)" :error="moment.error" :issues="moment.issues"
      @open="moment.open" @apply="moment.apply" @close="moment.close" @keep="moment.keepEditing" @discard="moment.discardPending" />`,
  })
  return { wrapper: mount(host), moment, editorText, busy, locale }
}

describe('片段工作台组件', () => {
  it.each([
    ['zh-CN', '选择内容', '选择素材'],
    ['en', 'Choose content', 'Choose an asset'],
  ] as const)('内部目标使用 %s 内容提示，素材选择保留素材提示', async (language, contentHint, assetHint) => {
    const { wrapper, locale } = mountMoment()
    locale.value = language
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    await wrapper.get('[data-testid="moment-target-kind"]').setValue('internal')
    expect(wrapper.get('[data-testid="moment-target-content"] option[value=""]').text()).toBe(contentHint)
    expect(wrapper.get('[data-testid="moment-asset"] option[value=""]').text()).toBe(assetHint)
  })

  it('默认首条与只读 ID，切换新增记录保留全部输入及原生编辑区隔离', async () => {
    const { wrapper, moment } = mountMoment()
    expect(wrapper.find('[data-testid="moment-fields"]').exists()).toBe(false)
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    expect(wrapper.get('[data-testid="moment-id"]').text()).toBe(fixture.moments[0]!.id)
    expect(wrapper.find('input[data-testid="moment-id"]').exists()).toBe(false)
    const firstInput = wrapper.get('[data-testid="moment-caption-zh"]').element
    await wrapper.get('[data-testid="moment-caption-zh"]').setValue('第一条保留')
    await wrapper.get('[data-testid="moment-add"]').trigger('click')
    const id = moment.draft!.moments.at(-1)!.id
    expect(wrapper.find('[data-testid="moment-caption-zh"]').exists()).toBe(false)
    await wrapper.get('[data-testid="moment-custom-caption"]').setValue(true)
    expect(wrapper.get('[data-testid="moment-caption-zh"]').element).not.toBe(firstInput)
    await wrapper.get('[data-testid="moment-caption-zh"]').setValue('新记录保留')
    await wrapper.get(`[data-testid="moment-record-${fixture.moments[0]!.id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="moment-caption-zh"]').element).toHaveProperty('value', '第一条保留')
    await wrapper.get(`[data-testid="moment-record-${id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="moment-caption-zh"]').element).toHaveProperty('value', '新记录保留')
    expect(wrapper.find('[data-testid="moment-discard-prompt"]').exists()).toBe(false)
  })
  it('显式启用可选文本，清空英文与关闭删除属性，目标切换清除旧分支', async () => {
    const { wrapper, moment, locale } = mountMoment()
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    await wrapper.get('[data-testid="moment-custom-caption"]').setValue(false)
    expect(moment.draft!.moments[0]).not.toHaveProperty('caption')
    await wrapper.get('[data-testid="moment-custom-caption"]').setValue(true)
    expect(moment.draft!.moments[0]!.caption).toEqual({ 'zh-CN': '' })
    await wrapper.get('[data-testid="moment-caption-zh"]').setValue('中文')
    await wrapper.get('[data-testid="moment-caption-en"]').setValue('English')
    await wrapper.get('[data-testid="moment-caption-en"]').setValue('')
    expect(moment.draft!.moments[0]!.caption).not.toHaveProperty('en')
    await wrapper.get('[data-testid="moment-target-kind"]').setValue('internal')
    await wrapper.get('[data-testid="moment-target-content"]').setValue('release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff')
    expect(moment.draft!.moments[0]!.target).toEqual({ kind: 'internal', contentId: 'release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff' })
    await wrapper.get('[data-testid="moment-add"]').trigger('click')
    const id = moment.draft!.moments.at(-1)!.id
    await wrapper.get('[data-testid="moment-target-kind"]').setValue('internal')
    expect(wrapper.get('[data-testid="moment-target-content"]').findAll('option').map(option => option.attributes('value'))).toContain(id)
    for (const value of ['hero_7cfbd2b5-69b4-41f6-9dbd-275c587b70cb', 'release_5081a10a-7f9a-4af5-a7dc-2b522e1d3cff', 'track_e60c46c7-28d6-418e-8905-167307dc8354', 'video_b634e94d-b468-4e4b-b784-22eda2c49314', 'event_dc200c4c-abbb-476e-984d-405470f2dbea', 'moment_c8f0a2c9-7b1f-4bcc-a6c0-e6d5e74538c8', 'artist_eb908750-faf8-418f-94c6-97f769c1cf31']) {
      expect(wrapper.get('[data-testid="moment-target-content"]').findAll('option').map(option => option.attributes('value'))).toContain(value)
    }
    await wrapper.get('[data-testid="moment-target-kind"]').setValue('external')
    expect(moment.draft!.moments.at(-1)!.target).toEqual({ kind: 'external', link: { provider: 'website', url: '' } })
    await wrapper.get('[data-testid="moment-target-provider"]').setValue('bilibili')
    await wrapper.get('[data-testid="moment-target-url"]').setValue('https://example.com/moment')
    await wrapper.get('[data-testid="moment-target-custom-label"]').setValue(true)
    await wrapper.get('[data-testid="moment-target-label-zh"]').setValue('查看')
    await wrapper.get('[data-testid="moment-target-label-en"]').setValue('Open')
    await wrapper.get('[data-testid="moment-target-label-en"]').setValue('')
    expect(moment.draft!.moments.at(-1)!.target).toEqual({ kind: 'external', link: { provider: 'bilibili', url: 'https://example.com/moment', label: { 'zh-CN': '查看' } } })
    await wrapper.get('[data-testid="moment-target-custom-label"]').setValue(false)
    expect(moment.draft!.moments.at(-1)!.target).toEqual({ kind: 'external', link: { provider: 'bilibili', url: 'https://example.com/moment' } })
    expect(wrapper.get('[data-testid="moment-target-url"]').attributes('type')).toBe('url')
    await wrapper.get('[data-testid="moment-target-kind"]').setValue('none')
    expect(moment.draft!.moments.at(-1)).not.toHaveProperty('target')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-add"]').attributes('aria-label')).toBe('Add moment')
    expect(wrapper.get('[data-testid="moment-target-kind"]').text()).toContain('Internal')
  })
  it('素材仅图片/GIF，非法 JSON 保留最近有效选项与输入，确认取消保持草稿', async () => {
    const { wrapper, editorText } = mountMoment()
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    const ids = wrapper.get('[data-testid="moment-asset"]').findAll('option').map(option => option.attributes('value'))
    for (const asset of fixture.assets) expect(ids.includes(asset.id)).toBe(['image', 'gif'].includes(asset.kind))
    expect(ids).toContain('asset_a3c8a851-1654-46e3-9bb7-a8640b852c3f')
    expect(wrapper.find('img, video, audio').exists()).toBe(false)
    await wrapper.get('[data-testid="moment-caption-zh"]').setValue('保留')
    editorText.value = '{'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-caption-zh"]').element).toHaveProperty('value', '保留')
    expect(wrapper.get('[data-testid="moment-asset"]').findAll('option').map(option => option.attributes('value'))).toEqual(ids)
    expect(wrapper.get('[data-testid="moment-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="moment-close"]').trigger('click')
    await wrapper.get('[data-testid="moment-keep"]').trigger('click')
    expect(wrapper.find('[data-testid="moment-fields"]').exists()).toBe(true)
    await wrapper.get('[data-testid="moment-close"]').trigger('click')
    await wrapper.get('[data-testid="moment-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="moment-fields"]').exists()).toBe(false)
  })
  it('诊断最多八条并本地化剩余数量，busy 锁定 fieldset 与全部按钮', async () => {
    const { wrapper, moment, busy, locale } = mountMoment()
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    moment.draft!.moments[0]!.assetId = 'asset_71734f61-5250-4825-a8b3-0192448be437'
    moment.apply()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-error"]').text()).toContain('/moments/0/assetId')
    expect(wrapper.get('[data-testid="moment-error"]').text()).toContain('asset-kind')
    moment.issues = Array.from({ length: 11 }, (_, index) => ({ path: `/moments/${index}/title`, source: 'schema' as const, code: 'required' }))
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-error"]').findAll('li')).toHaveLength(8)
    expect(wrapper.get('[data-testid="moment-error"]').text()).toContain('另有 3 项错误')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-error"]').text()).toContain('3 more issues')
    await wrapper.get('[data-testid="moment-open"]').trigger('click')
    busy.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="moment-fields"]').attributes('disabled')).toBeDefined()
    for (const button of wrapper.findAll('button')) expect(button.attributes('disabled') !== undefined || Boolean(button.element.closest('fieldset[disabled]'))).toBe(true)
  })
})
