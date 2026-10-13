import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { useEventWorkspace } from '../../composables/useEventWorkspace'

/** 以真实 JSON 运行状态机和应用事务。 */
function workspace() {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  return { editorText, busy, event: useEventWorkspace({ editorText, busy }) }
}
describe('现场草稿状态', () => {
  it('同步使用最新有效原文建立基线，不依赖旧解析结果', () => {
    const { event, editorText } = workspace()
    const source = structuredClone(fixture)
    source.events[0]!.title['zh-CN'] = '最新原文'
    editorText.value = JSON.stringify(source)
    expect(event.open()).toBe(true)
    expect(event.draft.value!.events[0]!.title['zh-CN']).toBe('最新原文')
    expect(event.stale.value).toBe(false)
  })

  it('仅显式应用更新唯一内容源，成功刷新基线', () => {
    const { event, editorText } = workspace()
    const before = editorText.value
    expect(event.open()).toBe(true)
    event.draft.value!.events[0]!.title['zh-CN'] = '已应用'
    expect(event.dirty.value).toBe(true)
    expect(editorText.value).toBe(before)
    expect(event.apply()).toBe(true)
    expect(JSON.parse(editorText.value).events[0].title['zh-CN']).toBe('已应用')
    expect(event.dirty.value).toBe(false)
    expect(event.stale.value).toBe(false)
  })
  it('重载和关闭脏输入需确认，取消保留，确认重载与关闭生效', () => {
    const { event } = workspace()
    event.open()
    event.draft.value!.events[0]!.title['zh-CN'] = '保留'
    expect(event.open()).toBe(false)
    expect(event.pending.value).toBe('open')
    event.keepEditing()
    expect(event.draft.value!.events[0]!.title['zh-CN']).toBe('保留')
    event.close()
    event.keepEditing()
    expect(event.pending.value).toBeNull()
    event.open()
    event.discardPending()
    expect(event.dirty.value).toBe(false)
    event.close()
    expect(event.draft.value).toBeNull()
    event.open()
    event.draft.value!.events[0]!.title['zh-CN'] = '放弃'
    event.close()
    event.discardPending()
    expect(event.draft.value).toBeNull()
  })
  it('过期拒绝覆盖，非法 JSON 重载失败保留草稿与待办', () => {
    const { event, editorText } = workspace()
    event.open()
    event.draft.value!.events[0]!.title['zh-CN'] = '保留'
    editorText.value = '{'
    expect(event.stale.value).toBe(true)
    expect(event.apply()).toBe(false)
    expect(event.error.value).toBe('stale-snapshot')
    event.open()
    event.discardPending()
    expect(event.error.value).toBe('invalid-snapshot')
    expect(event.pending.value).toBe('open')
    expect(event.draft.value!.events[0]!.title['zh-CN']).toBe('保留')
    expect(editorText.value).toBe('{')
  })
  it('非法字段应用失败保留输入与精确诊断', () => {
    const { event, editorText } = workspace()
    event.open()
    event.draft.value!.events[0]!.posterAssetId = 'asset_preview_sample'
    expect(event.apply()).toBe(false)
    expect(event.issues.value).toContainEqual(expect.objectContaining({ path: '/events/0/posterAssetId', code: 'asset-kind' }))
    expect(event.draft.value!.events[0]!.posterAssetId).toBe('asset_preview_sample')
    expect(editorText.value).toBe(JSON.stringify(fixture))
  })
  it('忙碌锁定所有变更操作且保留待办，reset 清理上下文', () => {
    const { event, busy } = workspace()
    expect(event.apply()).toBe(false)
    event.open()
    event.draft.value!.events[0]!.title['zh-CN'] = '保留'
    event.open()
    busy.value = true
    expect(event.open()).toBe(false)
    expect(event.apply()).toBe(false)
    event.close()
    event.discardPending()
    expect(event.pending.value).toBe('open')
    expect(event.draft.value!.events[0]!.title['zh-CN']).toBe('保留')
    event.reset()
    expect(event.draft.value).toBeNull()
    expect(event.pending.value).toBeNull()
    expect(event.error.value).toBeNull()
    expect(event.issues.value).toEqual([])
  })
})
