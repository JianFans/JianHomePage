import { describe, expect, it } from 'vitest'
import { reactive, ref } from 'vue'
import fixture from '../../../../content/fixtures/homepage.json'
import { useSiteWorkspace } from '../../composables/useSiteWorkspace'

describe('站点资料编辑状态', () => {
  it('保存失败保留输入，过期基线不能覆盖其他入口的修改', () => {
    const editorText = ref(JSON.stringify(fixture))
    const busy = ref(false)
    const state = reactive(useSiteWorkspace({ editorText, busy }))
    expect(state.open()).toBe(true)
    state.draft!.site.brand['zh-CN'] = '新站点'
    expect(state.dirty).toBe(true)
    busy.value = true
    expect(state.apply()).toBe(false)
    busy.value = false
    editorText.value += ' '
    expect(state.stale).toBe(true)
    expect(state.apply()).toBe(false)
    expect(state.draft!.site.brand['zh-CN']).toBe('新站点')
    expect(state.error).toBe('stale-snapshot')
    state.reset()
    expect(state.open()).toBe(true)
    state.draft!.artist.shortBio['zh-CN'] = '新简介'
    expect(state.apply()).toBe(true)
    expect(state.dirty).toBe(false)
    expect(JSON.parse(editorText.value).artist.shortBio['zh-CN']).toBe('新简介')
  })
})
