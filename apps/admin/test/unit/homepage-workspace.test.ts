import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { useHomepageWorkspace } from '../../composables/useHomepageWorkspace'

/** 使用实际快照文本验证首页草稿，避免状态桩掩盖应用事务。 */
function workspace() {
  const editorText = ref(JSON.stringify(fixture))
  const busy = ref(false)
  return { editorText, busy, home: useHomepageWorkspace({ editorText, busy }) }
}

describe('首页草稿状态', () => {
  it('仅显式应用完整有效草稿后更新唯一内容源', () => {
    const { home, editorText } = workspace()
    const initial = editorText.value
    expect(home.open()).toBe(true)
    expect(home.dirty.value).toBe(false)
    home.draft.value!.heroSlides[0]!.headline['zh-CN'] = '新宣传标题'
    expect(home.dirty.value).toBe(true)
    expect(editorText.value).toBe(initial)
    expect(home.apply()).toBe(true)
    expect(JSON.parse(editorText.value).heroSlides[0].headline['zh-CN']).toBe('新宣传标题')
    expect(home.dirty.value).toBe(false)
    expect(home.stale.value).toBe(false)
  })

  it('重载与关闭脏表单都需确认，继续编辑保留输入', () => {
    const { home } = workspace()
    home.open()
    home.draft.value!.heroSlides[0]!.headline['zh-CN'] = '保留输入'
    expect(home.open()).toBe(false)
    expect(home.pending.value).toBe('open')
    home.keepEditing()
    expect(home.draft.value!.heroSlides[0]!.headline['zh-CN']).toBe('保留输入')
    home.close()
    expect(home.pending.value).toBe('close')
    home.discardPending()
    expect(home.draft.value).toBeNull()
  })

  it('过期表单拒绝覆盖最新文本，明确重载后重新建立基线', () => {
    const { home, editorText } = workspace()
    home.open()
    home.draft.value!.heroSlides[0]!.headline['zh-CN'] = '旧输入'
    editorText.value += '\n'
    expect(home.stale.value).toBe(true)
    expect(home.apply()).toBe(false)
    expect(home.error.value).toBe('stale-snapshot')
    home.open()
    home.discardPending()
    expect(home.stale.value).toBe(false)
    expect(home.dirty.value).toBe(false)
  })

  it('非法草稿保留原文、输入和精确诊断', () => {
    const { home, editorText } = workspace()
    const initial = editorText.value
    home.open()
    home.draft.value!.heroSlides[0]!.assetId = 'asset_71734f61-5250-4825-a8b3-0192448be437'
    expect(home.apply()).toBe(false)
    expect(home.error.value).toBe('invalid-draft')
    expect(home.issues.value).toContainEqual(expect.objectContaining({ path: '/heroSlides/0/assetId', code: 'asset-kind' }))
    expect(editorText.value).toBe(initial)
    expect(home.draft.value!.heroSlides[0]!.assetId).toBe('asset_71734f61-5250-4825-a8b3-0192448be437')
  })

  it('busy 阻止打开、应用、关闭与确认，不清除待办', () => {
    const { home, busy } = workspace()
    home.open()
    home.draft.value!.heroSlides[0]!.headline['zh-CN'] = '保留'
    home.open()
    busy.value = true
    expect(home.open()).toBe(false)
    expect(home.apply()).toBe(false)
    home.close()
    home.discardPending()
    expect(home.pending.value).toBe('open')
    expect(home.draft.value!.heroSlides[0]!.headline['zh-CN']).toBe('保留')
  })

  it('重载失败不丢失旧表单，清理状态移除全部上下文', () => {
    const { home, editorText } = workspace()
    home.open()
    home.draft.value!.heroSlides[0]!.headline['zh-CN'] = '保留'
    home.open()
    editorText.value = '{'
    home.discardPending()
    expect(home.error.value).toBe('invalid-snapshot')
    expect(home.pending.value).toBe('open')
    expect(home.draft.value!.heroSlides[0]!.headline['zh-CN']).toBe('保留')
    home.reset()
    expect(home.draft.value).toBeNull()
    expect(home.pending.value).toBeNull()
    expect(home.error.value).toBeNull()
    expect(home.apply()).toBe(false)
    expect(home.open()).toBe(false)
  })
})
