import { effectScope, computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import { useMusicWorkspace } from '../../composables/useMusicWorkspace'
import { analyzeSnapshotText } from '../../utils/snapshot-workbench'

/** 在真实 Vue 作用域中创建工作区，使用完整 fixture 作为编辑基线。 */
function workspace() {
  const scope = effectScope()
  const editorText = ref(JSON.stringify(fixtureData))
  const busy = ref(false)
  const snapshot = computed(() => analyzeSnapshotText(editorText.value).snapshot)
  const music = scope.run(() => useMusicWorkspace({ editorText, busy, snapshot }))!
  return { scope, editorText, busy, music }
}

describe('音乐表单状态', () => {
  it('打开副本、应用成功后清除表单脏状态', () => {
    const { scope, music, editorText } = workspace()
    expect(music.select('release_01')).toBe(true)
    expect(music.dirty.value).toBe(false)
    music.draft.value!.release.title['zh-CN'] = '修改标题'
    expect(music.dirty.value).toBe(true)
    expect(JSON.parse(editorText.value).releases[0].title['zh-CN']).not.toBe('修改标题')
    expect(music.apply()).toBe(true)
    expect(JSON.parse(editorText.value).releases[0].title['zh-CN']).toBe('修改标题')
    expect(music.dirty.value).toBe(false)
    scope.stop()
  })

  it('切换作品前保留未应用输入，允许继续编辑或明确放弃', () => {
    const { scope, music } = workspace()
    music.select('release_01')
    music.draft.value!.release.title['zh-CN'] = '未应用'
    expect(music.select('release_02')).toBe(false)
    expect(music.pending.value).toMatchObject({ kind: 'select', id: 'release_02' })
    music.keepEditing()
    expect(music.pending.value).toBeNull()
    expect(music.draft.value!.release.title['zh-CN']).toBe('未应用')
    music.select('release_02')
    music.discardPending()
    expect(music.draft.value!.release.id).toBe('release_02')
    expect(music.dirty.value).toBe(false)
    scope.stop()
  })

  it('新增空表单也视为未应用，取消只放弃临时输入', () => {
    const { scope, music, editorText } = workspace()
    const initial = editorText.value
    music.select(null)
    expect(music.dirty.value).toBe(true)
    music.close()
    expect(music.pending.value?.kind).toBe('close')
    music.discardPending()
    expect(music.draft.value).toBeNull()
    expect(editorText.value).toBe(initial)
    scope.stop()
  })

  it('基线变化时拒绝应用，保留输入并允许重新载入', () => {
    const { scope, music, editorText } = workspace()
    music.select('release_01')
    music.draft.value!.release.title['zh-CN'] = '未应用'
    editorText.value += '\n'
    expect(music.stale.value).toBe(true)
    expect(music.apply()).toBe(false)
    expect(music.error.value).toBe('stale-snapshot')
    expect(music.draft.value!.release.title['zh-CN']).toBe('未应用')
    music.select('release_01')
    music.discardPending()
    expect(music.stale.value).toBe(false)
    scope.stop()
  })

  it('忙碌期间阻止切换、应用和丢弃操作', () => {
    const { scope, music, busy } = workspace()
    music.select('release_01')
    music.draft.value!.release.title['zh-CN'] = '保留'
    music.close()
    busy.value = true
    expect(music.select(null)).toBe(false)
    expect(music.apply()).toBe(false)
    music.discardPending()
    expect(music.draft.value!.release.title['zh-CN']).toBe('保留')
    scope.stop()
  })

  it('打开直接检查最新 JSON，不使用旧的有效快照', () => {
    const { scope, music, editorText } = workspace()
    editorText.value = '{'
    expect(music.select(null)).toBe(false)
    expect(music.error.value).toBe('invalid-snapshot')
    expect(music.draft.value).toBeNull()
    expect(music.apply()).toBe(false)
    scope.stop()
  })

  it('无效表单保留原文并返回准确问题路径', () => {
    const { scope, music, editorText } = workspace()
    const initial = editorText.value
    music.select('release_01')
    music.draft.value!.release.coverAssetId = 'asset_preview_sample'
    expect(music.apply()).toBe(false)
    expect(music.issues.value).toContainEqual(expect.objectContaining({ path: '/releases/0/coverAssetId', code: 'asset-kind' }))
    expect(editorText.value).toBe(initial)
    music.reset()
    expect(music.draft.value).toBeNull()
    scope.stop()
  })
})
