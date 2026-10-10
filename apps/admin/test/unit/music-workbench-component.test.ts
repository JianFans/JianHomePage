import { mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import MusicWorkbench from '../../components/MusicWorkbench.vue'
import { useMusicWorkspace } from '../../composables/useMusicWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

/** 挂载真实表单与状态层，所有断言针对最终快照和可操作字段。 */
function mountMusic() {
  const editorText = ref(JSON.stringify(fixtureData))
  const busy = ref(false)
  const locale = ref<'zh-CN' | 'en'>('zh-CN')
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const host = defineComponent({
    components: { MusicWorkbench },
    /** 暴露实际工作台状态，避免用 stub 隐藏表单数据流。 */
    setup() {
      const music = reactive(useMusicWorkspace({ editorText, busy, snapshot }))
      return { music, snapshot, busy, locale }
    },
    template: `<MusicWorkbench v-model:draft="music.draft" :snapshot="snapshot" :locale="locale" api-base-url="http://127.0.0.1:8080/api"
      :busy="busy" :dirty="music.dirty" :stale="music.stale" :error="music.error" :issues="music.issues"
      :pending="Boolean(music.pending)" @select="music.select" @apply="music.apply" @close="music.close"
      @keep="music.keepEditing" @discard="music.discardPending" />`,
  })
  return { wrapper: mount(host), editorText, busy, locale }
}

describe('音乐工作台界面', () => {
  it('在作品列表按当前语言显示作品类型', async () => {
    const { wrapper, locale } = mountMusic()
    expect(wrapper.get('[data-testid="music-release-release_01"] small').text()).toContain('单曲')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="music-release-release_01"] small').text()).toContain('Single')
  })

  it('为本地封面使用 API Origin 预览，不更改快照地址', async () => {
    const { wrapper, editorText } = mountMusic()
    expect(wrapper.get('[data-testid="music-release-release_01"] img').attributes('src')).toMatch(/^http:\/\/127\.0\.0\.1:8080\/media\//)
    expect(JSON.parse(editorText.value).assets[0].src).toBe(fixtureData.assets[0]!.src)
  })

  it('编辑平台标签、署名并移除试听时保持可选字段契约', async () => {
    const { wrapper, editorText } = mountMusic()
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    const track = wrapper.get('[data-testid="track-editor"]')
    await track.get('[data-testid="track-preview"]').setValue('')
    const links = wrapper.findAll('[data-testid="platform-link"]')
    await links[0]!.get('input[type="checkbox"]').setValue(true)
    await links[0]!.get('input[type="text"]').setValue('听歌')
    await track.get('[data-testid="credit-name"]').setValue('新署名')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    const next = JSON.parse(editorText.value)
    expect(next.tracks[0].previewAssetId).toBeUndefined()
    expect(next.tracks[0].previewDurationSeconds).toBeUndefined()
    expect(next.tracks[0].credits[0].name).toBe('新署名')
    expect(next.releases[0].platformLinks[0].label).toEqual({ 'zh-CN': '听歌' })
  })

  it('选择作品、筛选素材并应用双语标题', async () => {
    const { wrapper, editorText } = mountMusic()
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    const cover = wrapper.get('[data-testid="music-cover"]')
    const preview = wrapper.get('[data-testid="track-preview"]').findAll('option')
    expect(cover.findAll('option').some(option => option.attributes('value') === 'asset_preview_sample')).toBe(false)
    expect(preview.some(option => option.attributes('value') === 'asset_preview_sample')).toBe(true)
    expect(preview.some(option => option.attributes('value') === 'asset_cover_01')).toBe(false)
    await wrapper.get('[data-testid="release-title-zh"]').setValue('新的标题')
    await wrapper.get('[data-testid="release-title-en"]').setValue('New title')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    expect(JSON.parse(editorText.value).releases[0].title).toEqual({ 'zh-CN': '新的标题', en: 'New title' })
    expect(wrapper.get('[data-testid="music-apply"]').attributes('disabled')).toBeDefined()
  })

  it('添加并重排曲目，清空英文时移除选填键', async () => {
    const { wrapper, editorText } = mountMusic()
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    await wrapper.get('[data-testid="release-title-en"]').setValue('')
    await wrapper.get('[data-testid="track-add"]').trigger('click')
    const tracks = wrapper.findAll('[data-testid="track-editor"]')
    await tracks[1]!.get('[data-testid="track-title-zh"]').setValue('第二首')
    await tracks[1]!.get('[data-testid="track-duration"]').setValue(200)
    await tracks[1]!.get('[data-testid="track-up"]').trigger('click')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    const next = JSON.parse(editorText.value)
    expect(next.releases[0].title.en).toBeUndefined()
    expect(next.releases[0].trackIds[0]).not.toBe('track_01')
    expect(next.tracks.find((track: { title: { 'zh-CN': string } }) => track.title['zh-CN'] === '第二首').durationSeconds).toBe(200)
  })

  it('重复曲目引用共享字段，移除一次引用后仍保留曲目记录', async () => {
    const { wrapper, editorText } = mountMusic()
    const value = structuredClone(fixtureData)
    value.releases[0]!.trackIds.push(value.releases[0]!.trackIds[0]!)
    editorText.value = JSON.stringify(value)
    await wrapper.vm.$nextTick()
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    const tracks = wrapper.findAll('[data-testid="track-editor"]')
    await tracks[1]!.get('[data-testid="track-title-zh"]').setValue('共享曲目标题')
    expect(tracks[0]!.get('[data-testid="track-title-zh"]').element).toHaveProperty('value', '共享曲目标题')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    expect(wrapper.find('[data-testid="music-error"]').exists()).toBe(false)
    expect(JSON.parse(editorText.value).tracks[0].title['zh-CN']).toBe('共享曲目标题')
    expect(JSON.parse(editorText.value).releases[0].trackIds).toEqual(['track_01', 'track_01'])
    await wrapper.findAll('[data-testid="track-remove"]')[1]!.trigger('click')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    const next = JSON.parse(editorText.value)
    expect(next.releases[0].trackIds).toEqual(['track_01'])
    expect(next.tracks.filter((track: { id: string }) => track.id === 'track_01')).toHaveLength(1)
    expect(next.tracks[0].title['zh-CN']).toBe('共享曲目标题')
    expect(analyzeSnapshotText(editorText.value).issues).toEqual([])
  })

  it('移出音乐条目和重新排序不会更改板块限制', async () => {
    const { wrapper, editorText } = mountMusic()
    await wrapper.get('[data-testid="music-release-release_02"]').trigger('click')
    await wrapper.get('[data-testid="section-section_music"] [data-testid="section-up"]').trigger('click')
    await wrapper.get('[data-testid="music-apply"]').trigger('click')
    const section = JSON.parse(editorText.value).homepage.sections.find((item: { type: string }) => item.type === 'music')
    expect(section.itemIds[0]).toBe('release_02')
    expect(section.limit).toBe(5)
  })

  it('未应用切换显示内联确认，取消保留 JSON', async () => {
    const { wrapper, editorText } = mountMusic()
    const original = editorText.value
    await wrapper.get('[data-testid="music-new"]').trigger('click')
    await wrapper.get('[data-testid="music-close"]').trigger('click')
    expect(wrapper.find('[data-testid="music-discard-prompt"]').exists()).toBe(true)
    await wrapper.get('[data-testid="music-keep"]').trigger('click')
    expect(wrapper.find('[data-testid="release-title-zh"]').exists()).toBe(true)
    await wrapper.get('[data-testid="music-close"]').trigger('click')
    await wrapper.get('[data-testid="music-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="release-title-zh"]').exists()).toBe(false)
    expect(editorText.value).toBe(original)
  })

  it('切换语言不清空内容，忙碌期间禁用表单和新建', async () => {
    const { wrapper, busy, locale } = mountMusic()
    await wrapper.get('[data-testid="music-release-release_01"]').trigger('click')
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="music-new"]').attributes('aria-label')).toBe('New release')
    expect(wrapper.get('[data-testid="release-title-zh"]').element).toHaveProperty('value', '示例作品 01')
    busy.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="music-new"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-testid="music-fields"]').attributes('disabled')).toBeDefined()
  })
})
