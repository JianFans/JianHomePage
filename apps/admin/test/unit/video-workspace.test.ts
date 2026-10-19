import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { useVideoWorkspace } from '../../composables/useVideoWorkspace'

/** 以真实 JSON 运行状态机和应用事务。 */
function workspace() {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  return { editorText, busy, video: useVideoWorkspace({ editorText, busy }) }
}
describe('影像草稿状态', () => {
  it('仅显式应用更新唯一内容源，成功刷新基线', () => {
    const { video, editorText } = workspace()
    const before = editorText.value
    expect(video.open()).toBe(true)
    video.draft.value!.videos[0]!.title['zh-CN'] = '已应用'
    expect(video.dirty.value).toBe(true)
    expect(editorText.value).toBe(before)
    expect(video.apply()).toBe(true)
    expect(JSON.parse(editorText.value).videos[0].title['zh-CN']).toBe('已应用')
    expect(video.dirty.value).toBe(false)
    expect(video.stale.value).toBe(false)
  })
  it('重载和关闭脏输入需确认，取消保留，确认重载与关闭生效', () => {
    const { video } = workspace()
    video.open()
    video.draft.value!.videos[0]!.title['zh-CN'] = '保留'
    expect(video.open()).toBe(false)
    expect(video.pending.value).toBe('open')
    video.keepEditing()
    expect(video.draft.value!.videos[0]!.title['zh-CN']).toBe('保留')
    video.close()
    video.keepEditing()
    expect(video.pending.value).toBeNull()
    video.open()
    video.discardPending()
    expect(video.dirty.value).toBe(false)
    video.close()
    expect(video.draft.value).toBeNull()
    video.open()
    video.draft.value!.videos[0]!.title['zh-CN'] = '放弃'
    video.close()
    video.discardPending()
    expect(video.draft.value).toBeNull()
  })
  it('过期拒绝覆盖，非法 JSON 重载失败保留草稿与待办', () => {
    const { video, editorText } = workspace()
    video.open()
    video.draft.value!.videos[0]!.title['zh-CN'] = '保留'
    editorText.value = '{'
    expect(video.stale.value).toBe(true)
    expect(video.apply()).toBe(false)
    expect(video.error.value).toBe('stale-snapshot')
    video.open()
    video.discardPending()
    expect(video.error.value).toBe('invalid-snapshot')
    expect(video.pending.value).toBe('open')
    expect(video.draft.value!.videos[0]!.title['zh-CN']).toBe('保留')
    expect(editorText.value).toBe('{')
  })
  it('非法字段应用失败保留输入与精确诊断', () => {
    const { video, editorText } = workspace()
    video.open()
    video.draft.value!.videos[0]!.posterAssetId = 'asset_71734f61-5250-4825-a8b3-0192448be437'
    expect(video.apply()).toBe(false)
    expect(video.issues.value).toContainEqual(expect.objectContaining({ path: '/videos/0/posterAssetId', code: 'asset-kind' }))
    expect(video.draft.value!.videos[0]!.posterAssetId).toBe('asset_71734f61-5250-4825-a8b3-0192448be437')
    expect(editorText.value).toBe(JSON.stringify(fixture))
  })
  it('忙碌锁定所有变更操作且保留待办，reset 清理上下文', () => {
    const { video, busy } = workspace()
    expect(video.apply()).toBe(false)
    video.open()
    video.draft.value!.videos[0]!.title['zh-CN'] = '保留'
    video.open()
    busy.value = true
    expect(video.open()).toBe(false)
    expect(video.apply()).toBe(false)
    video.close()
    video.discardPending()
    expect(video.pending.value).toBe('open')
    expect(video.draft.value!.videos[0]!.title['zh-CN']).toBe('保留')
    video.reset()
    expect(video.draft.value).toBeNull()
    expect(video.pending.value).toBeNull()
    expect(video.error.value).toBeNull()
    expect(video.issues.value).toEqual([])
  })
})
