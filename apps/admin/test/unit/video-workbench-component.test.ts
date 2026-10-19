import { mount } from '@vue/test-utils'
import { computed, defineComponent, reactive, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import VideoWorkbench from '../../components/VideoWorkbench.vue'
import { useVideoWorkspace } from '../../composables/useVideoWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

/** 挂载真实状态与组件，不用事件桩替代应用事务。 */
function mountVideo() {
  const source = structuredClone(fixture)
  source.assets.push({ ...source.assets[0]!, id: 'asset_a0b7ada8-6b3e-452f-bdf3-8fa5bcb0cb6c', kind: 'video', mimeType: 'video/mp4', src: '/media/test.mp4' })
  source.assets.push({ ...source.assets[0]!, id: 'asset_a3c8a851-1654-46e3-9bb7-a8640b852c3f', kind: 'gif', mimeType: 'image/gif', src: '/media/test.gif' })
  const editorText = ref(JSON.stringify(source))
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const busy = ref(false)
  const locale = ref<'zh-CN' | 'en'>('zh-CN')
  const video = reactive(useVideoWorkspace({ editorText, busy }))
  const host = defineComponent({
    components: { VideoWorkbench }, setup: () => ({ video, snapshot, busy, locale }),
    template: `<VideoWorkbench v-model:draft="video.draft" :snapshot="snapshot" :locale="locale" :busy="busy"
      :dirty="video.dirty" :stale="video.stale" :pending="Boolean(video.pending)" :error="video.error" :issues="video.issues"
      @open="video.open" @apply="video.apply" @close="video.close" @keep="video.keepEditing" @discard="video.discardPending" />`,
  })
  return { wrapper: mount(host), video, editorText, busy, locale }
}
describe('影像工作台组件', () => {
  it('首条默认选中，ID 只读；切换位置保留所有记录输入', async () => {
    const { wrapper, video } = mountVideo()
    expect(wrapper.find('[data-testid="video-fields"]').exists()).toBe(false)
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    expect(wrapper.get('[data-testid="video-id"]').text()).toBe(fixture.videos[0]!.id)
    expect(wrapper.find('input[data-testid="video-id"]').exists()).toBe(false)
    await wrapper.get('[data-testid="video-title-zh"]').setValue('第一条保留')
    await wrapper.get('[data-testid="video-add"]').trigger('click')
    const id = video.draft!.videos.at(-1)!.id
    await wrapper.get('[data-testid="video-title-zh"]').setValue('新记录保留')
    await wrapper.get(`[data-testid="video-record-${fixture.videos[0]!.id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '第一条保留')
    await wrapper.get(`[data-testid="video-record-${id}"]`).trigger('click')
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '新记录保留')
    expect(wrapper.find('[data-testid="video-discard-prompt"]').exists()).toBe(false)
  })
  it('双语、整数时长、精选、可选素材和平台链接绑定真实草稿', async () => {
    const { wrapper, video, locale } = mountVideo()
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    await wrapper.get('[data-testid="video-title-en"]').setValue('English')
    await wrapper.get('[data-testid="video-duration"]').setValue(183)
    await wrapper.get('[data-testid="video-featured"]').setValue(true)
    const media = { id: 'asset_a0b7ada8-6b3e-452f-bdf3-8fa5bcb0cb6c' }
    await wrapper.get('[data-testid="video-asset"]').setValue(media.id)
    expect(video.draft!.videos[0]).toMatchObject({ title: { en: 'English' }, durationSeconds: 183, featured: true, videoAssetId: media.id })
    await wrapper.get('[data-testid="video-title-en"]').setValue('')
    await wrapper.get('[data-testid="video-asset"]').setValue('')
    expect(video.draft!.videos[0]).not.toHaveProperty('videoAssetId')
    expect(video.draft!.videos[0]!.title).not.toHaveProperty('en')
    expect(wrapper.get('[data-testid="video-duration"]').attributes()).toMatchObject({ type: 'number', min: '1', step: '1' })
    await wrapper.get('[data-testid="link-add"]').trigger('click')
    await wrapper.findAll('[data-testid="link-provider"]').at(-1)!.setValue('bilibili')
    await wrapper.findAll('[data-testid="link-url"]').at(-1)!.setValue('https://www.bilibili.com/video/BV123456')
    expect(video.draft!.videos[0]!.platformLinks.at(-1)).toMatchObject({ provider: 'bilibili', url: 'https://www.bilibili.com/video/BV123456' })
    locale.value = 'en'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="video-add"]').attributes('aria-label')).toBe('Add video')
  })
  it('只提供图片/GIF 海报与视频素材，非法 JSON 保留最近有效选项和输入', async () => {
    const { wrapper, editorText } = mountVideo()
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    const posterIds = wrapper.get('[data-testid="video-poster"]').findAll('option').map(option => option.attributes('value'))
    const videoIds = wrapper.get('[data-testid="video-asset"]').findAll('option').map(option => option.attributes('value'))
    for (const asset of fixture.assets) {
      expect(posterIds.includes(asset.id)).toBe(['image', 'gif'].includes(asset.kind))
      expect(videoIds.includes(asset.id)).toBe(asset.kind === 'video')
    }
    expect(posterIds).toContain('asset_a3c8a851-1654-46e3-9bb7-a8640b852c3f')
    expect(posterIds).not.toContain('asset_a0b7ada8-6b3e-452f-bdf3-8fa5bcb0cb6c')
    expect(videoIds).toContain('asset_a0b7ada8-6b3e-452f-bdf3-8fa5bcb0cb6c')
    expect(videoIds).not.toContain('asset_a3c8a851-1654-46e3-9bb7-a8640b852c3f')
    await wrapper.get('[data-testid="video-title-zh"]').setValue('保留')
    editorText.value = '{'
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="video-title-zh"]').element).toHaveProperty('value', '保留')
    expect(wrapper.get('[data-testid="video-poster"]').findAll('option').map(option => option.attributes('value'))).toEqual(posterIds)
    expect(wrapper.get('[data-testid="video-apply"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="video-close"]').trigger('click')
    await wrapper.get('[data-testid="video-keep"]').trigger('click')
    expect(wrapper.find('[data-testid="video-fields"]').exists()).toBe(true)
    await wrapper.get('[data-testid="video-close"]').trigger('click')
    await wrapper.get('[data-testid="video-discard"]').trigger('click')
    expect(wrapper.find('[data-testid="video-fields"]').exists()).toBe(false)
  })
  it('展示精确诊断，忙碌锁定 fieldset 与所有按钮', async () => {
    const { wrapper, video, busy } = mountVideo()
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    video.draft!.videos[0]!.posterAssetId = 'asset_71734f61-5250-4825-a8b3-0192448be437'
    video.apply()
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="video-error"]').text()).toContain('/videos/0/posterAssetId')
    expect(wrapper.get('[data-testid="video-error"]').text()).toContain('asset-kind')
    await wrapper.get('[data-testid="video-open"]').trigger('click')
    busy.value = true
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-testid="video-fields"]').attributes('disabled')).toBeDefined()
    for (const button of wrapper.findAll('button')) expect(button.attributes('disabled') !== undefined || Boolean(button.element.closest('fieldset[disabled]'))).toBe(true)
  })
})
