import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { useMomentWorkspace } from '../../composables/useMomentWorkspace'

/** 以真实 JSON 运行状态机和应用事务。 */
function workspace() {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  return { editorText, busy, moment: useMomentWorkspace({ editorText, busy }) }
}
describe('片段草稿状态', () => {
  it('同步使用最新有效原文建立基线，不依赖旧解析结果', () => {
    const { moment, editorText } = workspace()
    const source = structuredClone(fixture)
    source.moments[0]!.caption!['zh-CN'] = '最新原文'
    editorText.value = JSON.stringify(source)
    expect(moment.open()).toBe(true)
    expect(moment.draft.value!.moments[0]!.caption!['zh-CN']).toBe('最新原文')
    expect(moment.stale.value).toBe(false)
  })

  it('仅显式应用更新唯一内容源，成功刷新基线', () => {
    const { moment, editorText } = workspace()
    const before = editorText.value
    expect(moment.open()).toBe(true)
    moment.draft.value!.moments[0]!.caption!['zh-CN'] = '已应用'
    expect(moment.dirty.value).toBe(true)
    expect(editorText.value).toBe(before)
    expect(moment.apply()).toBe(true)
    expect(JSON.parse(editorText.value).moments[0].caption!['zh-CN']).toBe('已应用')
    expect(moment.dirty.value).toBe(false)
    expect(moment.stale.value).toBe(false)
  })
  it('重载和关闭脏输入需确认，取消保留，确认重载与关闭生效', () => {
    const { moment } = workspace()
    moment.open()
    moment.draft.value!.moments[0]!.caption!['zh-CN'] = '保留'
    expect(moment.open()).toBe(false)
    expect(moment.pending.value).toBe('open')
    moment.keepEditing()
    expect(moment.draft.value!.moments[0]!.caption!['zh-CN']).toBe('保留')
    moment.close()
    moment.keepEditing()
    expect(moment.pending.value).toBeNull()
    moment.open()
    moment.discardPending()
    expect(moment.dirty.value).toBe(false)
    moment.close()
    expect(moment.draft.value).toBeNull()
    moment.open()
    moment.draft.value!.moments[0]!.caption!['zh-CN'] = '放弃'
    moment.close()
    moment.discardPending()
    expect(moment.draft.value).toBeNull()
  })
  it('过期拒绝覆盖，非法 JSON 重载失败保留草稿与待办', () => {
    const { moment, editorText } = workspace()
    moment.open()
    moment.draft.value!.moments[0]!.caption!['zh-CN'] = '保留'
    editorText.value = '{'
    expect(moment.stale.value).toBe(true)
    expect(moment.apply()).toBe(false)
    expect(moment.error.value).toBe('stale-snapshot')
    moment.open()
    moment.discardPending()
    expect(moment.error.value).toBe('invalid-snapshot')
    expect(moment.pending.value).toBe('open')
    expect(moment.draft.value!.moments[0]!.caption!['zh-CN']).toBe('保留')
    expect(editorText.value).toBe('{')
  })
  it('非法字段应用失败保留输入与精确诊断', () => {
    const { moment, editorText } = workspace()
    moment.open()
    moment.draft.value!.moments[0]!.assetId = 'asset_preview_sample'
    expect(moment.apply()).toBe(false)
    expect(moment.issues.value).toContainEqual(expect.objectContaining({ path: '/moments/0/assetId', code: 'asset-kind' }))
    expect(moment.draft.value!.moments[0]!.assetId).toBe('asset_preview_sample')
    expect(editorText.value).toBe(JSON.stringify(fixture))
  })
  it('忙碌锁定所有变更操作且保留待办，reset 清理上下文', () => {
    const { moment, busy } = workspace()
    expect(moment.apply()).toBe(false)
    moment.open()
    moment.draft.value!.moments[0]!.caption!['zh-CN'] = '保留'
    moment.open()
    busy.value = true
    expect(moment.open()).toBe(false)
    expect(moment.apply()).toBe(false)
    moment.close()
    moment.discardPending()
    expect(moment.pending.value).toBe('open')
    expect(moment.draft.value!.moments[0]!.caption!['zh-CN']).toBe('保留')
    moment.reset()
    expect(moment.draft.value).toBeNull()
    expect(moment.pending.value).toBeNull()
    expect(moment.error.value).toBeNull()
    expect(moment.issues.value).toEqual([])
  })
})
