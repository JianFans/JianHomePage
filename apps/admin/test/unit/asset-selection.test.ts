import { describe, expect, it } from 'vitest'
import { reactive, ref } from 'vue'
import fixture from '../../../../content/fixtures/homepage.json'
import { useVideoWorkspace } from '../../composables/useVideoWorkspace'
import { appendPickedAsset, canRebaseAssets, isAssetAppendOnly } from '../../utils/asset-selection'
import type { Asset } from '@yujian/schema'

const extra = { ...fixture.assets[0], id: 'asset_132b59fd-f9b4-47b4-acd1-bbfae60d66b1' } as Asset

describe('字段素材选择事务', () => {
  it('仅允许有草稿且基线和当前文本均匹配的安全素材追加重建基线', () => {
    const before = JSON.stringify(fixture)
    const after = appendPickedAsset(before, extra, ['image', 'gif']).text
    const draft = { title: '尚未保存的输入' }
    expect(canRebaseAssets(draft, before, after, before, after)).toBe(true)
    expect(canRebaseAssets(null, before, after, before, after)).toBe(false)
    expect(canRebaseAssets(draft, before + ' ', after, before, after)).toBe(false)
    expect(canRebaseAssets(draft, before, after + ' ', before, after)).toBe(false)
    expect(canRebaseAssets(draft, before, before, before, before)).toBe(false)
    const changed = JSON.parse(after)
    changed.site.brand['zh-CN'] = '其他内容变更'
    const unsafe = JSON.stringify(changed)
    expect(canRebaseAssets(draft, before, unsafe, before, unsafe)).toBe(false)
  })

  it('重复绑定忽略素材及嵌套对象的键顺序，但仍拒绝嵌套值变化', () => {
    const asset = fixture.assets[0] as Asset
    const reordered = Object.fromEntries(Object.entries(asset).reverse()) as Asset
    reordered.alt = Object.fromEntries(Object.entries(asset.alt).reverse()) as Asset['alt']
    reordered.rights = Object.fromEntries(Object.entries(asset.rights).reverse()) as Asset['rights']
    reordered.rights.source = Object.fromEntries(Object.entries(asset.rights.source).reverse()) as Asset['rights']['source']
    const text = JSON.stringify(fixture)
    expect(appendPickedAsset(text, reordered, [asset.kind])).toEqual({ text, error: null })
    expect(appendPickedAsset(text, { ...reordered, rights: { ...reordered.rights, source: { 'zh-CN': '不同来源' } } }, [asset.kind]).error).toBe('conflict')
  })

  it('追加完整素材并拒绝同 ID 元数据冲突及用途不符', () => {
    const text = JSON.stringify(fixture)
    const result = appendPickedAsset(text, extra, ['image', 'gif'])
    expect(result.error).toBeNull()
    expect(JSON.parse(result.text).assets.at(-1).id).toBe(extra.id)
    expect(isAssetAppendOnly(text, result.text)).toBe(true)
    expect(appendPickedAsset(text, extra, ['audio']).error).toBe('invalid-asset')
    expect(appendPickedAsset(text, { ...fixture.assets[0], src: 'https://other.example/changed.webp' } as Asset, ['image', 'gif']).error).toBe('conflict')
    const changed = JSON.parse(result.text)
    changed.site.brand['zh-CN'] = '其他变更'
    expect(isAssetAppendOnly(text, JSON.stringify(changed))).toBe(false)
    changed.site.brand = fixture.site.brand
    changed.assets[0].alt['zh-CN'] = '偷偷修改'
    expect(isAssetAppendOnly(text, JSON.stringify(changed))).toBe(false)
  })

  it('素材追加后保持影像临时输入，并继续拒绝其他快照变化', () => {
    const editorText = ref(JSON.stringify(fixture))
    const state = reactive(useVideoWorkspace({ editorText, busy: ref(false) }))
    state.open()
    state.draft!.videos[0]!.title['zh-CN'] = '尚未保存的标题'
    const before = editorText.value
    const next = appendPickedAsset(before, extra, ['image', 'gif']).text
    editorText.value = next
    expect(state.rebaseAssets(before, next)).toBe(true)
    expect(state.dirty).toBe(true)
    expect(state.stale).toBe(false)
    state.draft!.videos[0]!.posterAssetId = extra.id
    expect(state.apply()).toBe(true)
    expect(JSON.parse(editorText.value).videos[0].title['zh-CN']).toBe('尚未保存的标题')
    expect(JSON.parse(editorText.value).videos[0].posterAssetId).toBe(extra.id)
    const old = editorText.value
    editorText.value += ' '
    expect(state.rebaseAssets(before, editorText.value)).toBe(false)
    expect(state.stale).toBe(true)
    expect(old).not.toBe(editorText.value)
  })
})
