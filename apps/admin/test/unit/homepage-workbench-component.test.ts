import { mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import HomepageWorkbench from '../../components/HomepageWorkbench.vue'
import { useHomepageWorkspace } from '../../composables/useHomepageWorkspace'
import { createHomepageDraft, type HomepageDraft, type HomepageErrorCode } from '../../utils/homepage-workbench'
import { analyzeSnapshotText, type SnapshotEditorIssue } from '../../utils/snapshot-workbench'

/** 使用真实可变草稿挂载组件，检查用户操作实际写入的字段和引用位置。 */
function mountHomepage(open = true, snapshotData = fixtureData) {
  const snapshot = analyzeSnapshotText(JSON.stringify(snapshotData)).snapshot!
  const draft = ref<HomepageDraft | null>(open ? createHomepageDraft(snapshot) : null)
  const busy = ref(false)
  const locale = ref<'zh-CN' | 'en'>('zh-CN')
  const dirty = ref(true)
  const stale = ref(false)
  const pending = ref(false)
  const error = ref<HomepageErrorCode | null>(null)
  const issues = ref<SnapshotEditorIssue[]>([])
  const host = defineComponent({
    components: { HomepageWorkbench },
    setup: () => ({ draft, snapshot, busy, locale, dirty, stale, pending, error, issues }),
    template: `<HomepageWorkbench v-model:draft="draft" :snapshot="snapshot" :locale="locale"
      :busy="busy" :dirty="dirty" :stale="stale" :pending="pending" :error="error" :issues="issues" />`,
  })
  const wrapper = mount(host)
  return { wrapper, draft, busy, locale, dirty, stale, pending, error, issues }
}

describe('首页工作台界面', () => {
  it('关闭状态提供打开入口，打开后提供重载及关闭事件', async () => {
    const { wrapper, draft } = mountHomepage(false)
    const workbench = wrapper.getComponent(HomepageWorkbench)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    expect(workbench.emitted('open')).toHaveLength(1)
    expect(wrapper.find('[data-testid="homepage-fields"]').exists()).toBe(false)
    draft.value = createHomepageDraft(analyzeSnapshotText(JSON.stringify(fixtureData)).snapshot!)
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="homepage-close"]').trigger('click')
    expect(workbench.emitted('close')).toHaveLength(1)
  })

  it('初始选择首个宣传位并绑定双语，清空英文移除键', async () => {
    const { wrapper, draft, locale } = mountHomepage()
    expect(wrapper.get('[data-testid="homepage-hero-id"]').text()).toBe('hero_studio')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('新的宣传位')
    await wrapper.get('[data-testid="homepage-headline-en"]').setValue('New hero')
    expect(draft.value!.heroSlides[0]!.headline).toEqual({ 'zh-CN': '新的宣传位', en: 'New hero' })
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-hero-add"]').attributes('aria-label')).toBe('Add hero')
    await wrapper.get('[data-testid="homepage-headline-en"]').setValue('')
    expect(draft.value!.heroSlides[0]!.headline).not.toHaveProperty('en')
  })

  it('新增后选中新记录，删除只更新草稿并移除板块引用', async () => {
    const { wrapper, draft } = mountHomepage()
    await wrapper.get('[data-testid="homepage-hero-add"]').trigger('click')
    const newId = draft.value!.heroSlides.at(-1)!.id
    expect(wrapper.get('[data-testid="homepage-hero-id"]').text()).toBe(newId)
    ;(draft.value!.sections[0]!.itemIds as string[]).push(newId)
    await wrapper.get('[data-testid="homepage-hero-remove"]').trigger('click')
    expect(draft.value!.heroSlides.some(hero => hero.id === newId)).toBe(false)
    expect(draft.value!.sections[0]!.itemIds).not.toContain(newId)
    expect(wrapper.get('[data-testid="homepage-hero-id"]').text()).toBe('hero_release')
  })

  it('筛选媒体与海报选项，切换媒体类型保留旧引用待完整校验', async () => {
    const { wrapper, draft } = mountHomepage()
    const assetOptions = wrapper.get('[data-testid="homepage-hero-asset"]').findAll('option').map(option => option.attributes('value'))
    expect(assetOptions).toContain('asset_hero_studio')
    expect(assetOptions).not.toContain('asset_preview_sample')
    await wrapper.get('[data-testid="homepage-media-kind"]').setValue('video')
    expect(draft.value!.heroSlides[0]!.assetId).toBe('asset_hero_studio')
    expect(wrapper.get('[data-testid="homepage-hero-asset"]').findAll('option').some(option => option.attributes('value') === 'asset_cover_01')).toBe(false)
    expect(wrapper.get('[data-testid="homepage-poster-asset"]').findAll('option').some(option => option.attributes('value') === 'asset_cover_01')).toBe(true)
  })

  it('可选素材、关联作品和 UTC 时间清空时移除键，非法时间保留原文', async () => {
    const { wrapper, draft } = mountHomepage()
    await wrapper.get('[data-testid="homepage-mobile-asset"]').setValue('asset_hero_stage')
    await wrapper.get('[data-testid="homepage-poster-asset"]').setValue('asset_cover_01')
    await wrapper.get('[data-testid="homepage-release"]').setValue('release_01')
    await wrapper.get('[data-testid="homepage-starts-at"]').setValue('2026-09-01T02:03:04Z')
    await wrapper.get('[data-testid="homepage-ends-at"]').setValue('incomplete')
    const hero = draft.value!.heroSlides[0]!
    expect(hero).toMatchObject({ mobileAssetId: 'asset_hero_stage', posterAssetId: 'asset_cover_01', releaseId: 'release_01', startsAt: '2026-09-01T02:03:04Z', endsAt: 'incomplete' })
    expect(wrapper.get('[data-testid="homepage-starts-at"]').attributes('type')).toBe('text')
    expect(wrapper.text()).toContain('UTC ISO 8601')
    for (const id of ['mobile-asset', 'poster-asset', 'release', 'starts-at', 'ends-at']) await wrapper.get(`[data-testid="homepage-${id}"]`).setValue('')
    for (const key of ['mobileAssetId', 'posterAssetId', 'releaseId', 'startsAt', 'endsAt']) expect(hero).not.toHaveProperty(key)
  })

  it('维护焦点、自动播放和单个外链的自定义双语标签', async () => {
    const { wrapper, draft } = mountHomepage()
    await wrapper.get('[data-testid="homepage-focal-x"]').setValue(24)
    await wrapper.get('[data-testid="homepage-focal-y"]').setValue(76)
    await wrapper.get('[data-testid="homepage-autoplay"]').setValue(true)
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('external')
    await wrapper.get('[data-testid="homepage-target-provider"]').setValue('website')
    await wrapper.get('[data-testid="homepage-target-url"]').setValue('https://example.com/hero')
    await wrapper.get('[data-testid="homepage-target-custom-label"]').setValue(true)
    await wrapper.get('[data-testid="homepage-target-label-zh"]').setValue('查看')
    const hero = draft.value!.heroSlides[0]!
    expect(hero).toMatchObject({ focalPoint: { x: 24, y: 76 }, autoplay: true, target: { kind: 'external', link: { provider: 'website', url: 'https://example.com/hero', label: { 'zh-CN': '查看' } } } })
    await wrapper.get('[data-testid="homepage-target-custom-label"]').setValue(false)
    expect(hero.target!.kind === 'external' && hero.target!.link).not.toHaveProperty('label')
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('internal')
    await wrapper.get('[data-testid="homepage-target-content"]').setValue('release_01')
    expect(hero.target).toEqual({ kind: 'internal', contentId: 'release_01' })
    expect(wrapper.get('[data-testid="homepage-target-visibility"]').text()).toContain('展示')
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('none')
    expect(hero).not.toHaveProperty('target')
  })

  it('编排按位置移动与移除重复引用，不改板块身份或布局', async () => {
    const { wrapper, draft } = mountHomepage()
    const section = draft.value!.sections.find(section => section.type === 'music')!
    section.itemIds = ['release_01', 'release_01', 'release_02']
    await wrapper.vm.$nextTick()
    const panel = wrapper.get('[data-testid="homepage-section-section_music"]')
    await panel.findAll('[data-testid="homepage-item-up"]')[2]!.trigger('click')
    expect(section.itemIds).toEqual(['release_01', 'release_02', 'release_01'])
    await panel.findAll('[data-testid="homepage-item-remove"]')[2]!.trigger('click')
    await panel.get('[data-testid="homepage-item-add-select"]').setValue('release_01')
    await panel.get('[data-testid="homepage-item-add"]').trigger('click')
    expect(section.itemIds).toEqual(['release_01', 'release_02', 'release_01'])
    await panel.get('[data-testid="homepage-section-up"]').trigger('click')
    expect(draft.value!.sections[0]).toMatchObject({ id: 'section_music', type: 'music', layoutVariant: 'cover-reel' })
  })

  it('重复板块 ID 按位置重排时清空旧内容选择，嵌套字段编辑仍保留选择', async () => {
    const source = structuredClone(fixtureData)
    const hero = source.homepage.sections[0]!
    const music = source.homepage.sections[1]!
    hero.id = music.id
    source.homepage.sections = [music, hero, ...source.homepage.sections.slice(2)]
    expect(analyzeSnapshotText(JSON.stringify(source)).issues).toEqual([])
    const { wrapper, draft } = mountHomepage(true, source)
    const panels = wrapper.findAll('[data-testid="homepage-section-section_music"]')
    expect(panels).toHaveLength(2)
    await panels[0]!.get('[data-testid="homepage-item-add-select"]').setValue('release_01')
    await panels[0]!.get('[data-testid="homepage-section-enabled"]').setValue(false)
    expect(panels[0]!.get('[data-testid="homepage-item-add-select"]').element).toHaveProperty('value', 'release_01')
    expect(panels[0]!.get('[data-testid="homepage-item-add"]').attributes('disabled')).toBeUndefined()
    await panels[0]!.get('[data-testid="homepage-section-down"]').trigger('click')
    expect(draft.value!.sections.slice(0, 2).map(section => [section.id, section.type, section.layoutVariant])).toEqual([
      ['section_music', 'hero', 'immersive'], ['section_music', 'music', 'cover-reel'],
    ])
    const reordered = wrapper.findAll('[data-testid="homepage-section-section_music"]')
    for (const panel of reordered) {
      expect(panel.get('[data-testid="homepage-item-add-select"]').element).toHaveProperty('value', '')
      expect(panel.get('[data-testid="homepage-item-add"]').attributes('disabled')).toBeDefined()
    }
    await reordered[1]!.get('[data-testid="homepage-item-add-select"]').setValue('release_01')
    await reordered[1]!.get('[data-testid="homepage-item-add"]').trigger('click')
    expect(draft.value!.sections[0]!.itemIds).toEqual(hero.itemIds)
    expect(draft.value!.sections[1]!.itemIds).toEqual([...music.itemIds, 'release_01'])
  })

  it('限制宣传位为 1 至 5 个引用、艺人为最多一个，并展示各板块 limit 边界', async () => {
    const { wrapper, draft } = mountHomepage()
    const hero = draft.value!.sections[0]!
    hero.itemIds = ['hero_studio']
    await wrapper.vm.$nextTick()
    const heroPanel = wrapper.get('[data-testid="homepage-section-section_hero"]')
    expect(heroPanel.get('[data-testid="homepage-item-remove"]').attributes('disabled')).toBeDefined()
    hero.itemIds.push('hero_studio', 'hero_studio', 'hero_studio', 'hero_studio')
    await wrapper.vm.$nextTick()
    expect(heroPanel.get('[data-testid="homepage-item-add"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="homepage-section-section_artist"] [data-testid="homepage-item-add"]').attributes('disabled')).toBeDefined()
    for (const [type, max] of [['music', '8'], ['video', '5'], ['event', '6'], ['moment', '8']]) {
      const limit = wrapper.get(`[data-testid="homepage-section-section_${type}"] [data-testid="homepage-section-limit"]`)
      expect(limit.attributes()).toMatchObject({ min: '1', max })
    }
  })

  it('使用草稿和 generatedAt 展示停用、数量限制与时间窗口', async () => {
    const { wrapper, draft } = mountHomepage()
    const music = draft.value!.sections.find(section => section.type === 'music')!
    if (music.type === 'music') music.limit = 1
    draft.value!.heroSlides[0]!.startsAt = '2030-01-01T00:00:00Z'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-section-section_hero"]').text()).toContain('时间窗口外')
    const panel = wrapper.get('[data-testid="homepage-section-section_music"]')
    expect(panel.text()).toContain('超过展示上限')
    await panel.get('[data-testid="homepage-section-enabled"]').setValue(false)
    expect(panel.text()).toContain('板块未启用')
  })

  it('音乐更多链接显式启停并稳定绑定 provider、地址与选填标签', async () => {
    const { wrapper, draft } = mountHomepage()
    const panel = wrapper.get('[data-testid="homepage-section-section_music"]')
    await panel.get('[data-testid="homepage-more-enabled"]').setValue(false)
    const music = draft.value!.sections.find(section => section.type === 'music')!
    expect(music).not.toHaveProperty('moreLink')
    await panel.get('[data-testid="homepage-more-enabled"]').setValue(true)
    await panel.get('[data-testid="homepage-more-provider"]').setValue('website')
    await panel.get('[data-testid="homepage-more-url"]').setValue('https://example.com/more')
    expect(music).toMatchObject({ moreLink: { provider: 'website', url: 'https://example.com/more' } })
  })

  it('内部曲目目标跟随作品实际展示位置提示可见性', async () => {
    const { wrapper, draft } = mountHomepage()
    await wrapper.get('[data-testid="homepage-target-kind"]').setValue('internal')
    await wrapper.get('[data-testid="homepage-target-content"]').setValue('track_01')
    expect(draft.value!.heroSlides[0]!.target).toEqual({ kind: 'internal', contentId: 'track_01' })
    expect(wrapper.get('[data-testid="homepage-target-visibility"]').text()).toBe('展示')
    const music = draft.value!.sections.find(section => section.type === 'music')!
    music.enabled = false
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-target-visibility"]').text()).toBe('板块未启用')
  })

  it('显示本地化错误和完整 JSON Pointer 诊断，确认按钮只发出相应意图', async () => {
    const { wrapper, pending, error, issues } = mountHomepage()
    const workbench = wrapper.getComponent(HomepageWorkbench)
    error.value = 'invalid-draft'
    issues.value = [{ path: '/heroSlides/0/assetId', source: 'semantic', code: 'asset-kind' }]
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-error"]').text()).toContain('请检查字段与引用')
    expect(wrapper.get('[data-testid="homepage-error"]').text()).toContain('/heroSlides/0/assetId')
    pending.value = true
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="homepage-keep"]').trigger('click')
    await wrapper.get('[data-testid="homepage-discard"]').trigger('click')
    await wrapper.get('[data-testid="homepage-apply"]').trigger('click')
    expect(workbench.emitted('keep')).toHaveLength(1)
    expect(workbench.emitted('discard')).toHaveLength(1)
    expect(workbench.emitted('apply')).toHaveLength(1)
  })

  it('JSON 临时无效时仍保留表单，应用拒绝覆盖并可确认关闭', async () => {
    const editorText = ref(JSON.stringify(fixtureData))
    const busy = ref(false)
    const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
    const homepage = reactive(useHomepageWorkspace({ editorText, busy }))
    const host = {
      components: { HomepageWorkbench },
      setup: () => ({ homepage, snapshot, busy }),
      template: `<HomepageWorkbench v-model:draft="homepage.draft" :snapshot="snapshot" locale="zh-CN" :busy="busy"
        :dirty="homepage.dirty" :stale="homepage.stale" :pending="Boolean(homepage.pending)" :error="homepage.error" :issues="homepage.issues"
        @open="homepage.open" @apply="homepage.apply" @close="homepage.close" @keep="homepage.keepEditing" @discard="homepage.discardPending" />`,
    }
    const wrapper = mount(host)
    await wrapper.get('[data-testid="homepage-open"]').trigger('click')
    await wrapper.get('[data-testid="homepage-headline-zh"]').setValue('保留输入')
    editorText.value = '{'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-headline-zh"]').element).toHaveProperty('value', '保留输入')
    expect(wrapper.get('[data-testid="homepage-open"]').attributes('disabled')).toBeDefined()
    expect(homepage.apply()).toBe(false)
    expect(editorText.value).toBe('{')
    await wrapper.get('[data-testid="homepage-close"]').trigger('click')
    await wrapper.get('[data-testid="homepage-discard"]').trigger('click')
    expect(homepage.draft).toBeNull()
    expect(editorText.value).toBe('{')
  })

  it('忙碌时锁定 fieldset 与所有操作，过期及干净草稿不能应用', async () => {
    const { wrapper, busy, stale, dirty, pending } = mountHomepage()
    stale.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-apply"]').attributes('disabled')).toBeDefined()
    stale.value = false
    dirty.value = false
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-apply"]').attributes('disabled')).toBeDefined()
    pending.value = true
    busy.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="homepage-fields"]').attributes('disabled')).toBeDefined()
    for (const id of ['open', 'close', 'hero-add', 'keep', 'discard', 'apply']) expect(wrapper.get(`[data-testid="homepage-${id}"]`).attributes('disabled')).toBeDefined()
  })
})
