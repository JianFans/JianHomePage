import { describe, expect, it } from 'vitest'
import { diffSnapshots, formatDiffValue } from '../../utils/snapshot-diff'

describe('saved snapshot comparison', () => {
  it('aligns records by ID and reports a reorder separately from field changes', () => {
    const before = { releases: [{ id: 'a', title: { 'zh-CN': 'Old' } }, { id: 'b', title: { 'zh-CN': 'B' } }] }
    const after = { releases: [{ id: 'b', title: { 'zh-CN': 'B' } }, { id: 'a', title: { 'zh-CN': 'New' } }] }
    expect(diffSnapshots(before, after)).toEqual([{ group: 'releases', changes: [
      { kind: 'reordered', path: '/releases', beforePath: '/releases', afterPath: '/releases', before: ['a', 'b'], after: ['b', 'a'] },
      { kind: 'changed', path: '/releases/@a/title/zh-CN', beforePath: '/releases/0/title/zh-CN', afterPath: '/releases/1/title/zh-CN', before: 'Old', after: 'New' },
    ] }])
    expect(before.releases[0]!.title['zh-CN']).toBe('Old')
  })

  it('reports record and field additions and removals without false reorders', () => {
    const groups = diffSnapshots(
      { tracks: [{ id: 'old', title: 'Old' }, { id: 'kept', previewAssetId: 'audio' }] },
      { tracks: [{ id: 'new', title: 'New' }, { id: 'kept', credits: [] }] },
    )
    expect(groups[0]!.changes.map(({ kind, path }) => ({ kind, path }))).toEqual([
      { kind: 'removed', path: '/tracks/@old' },
      { kind: 'added', path: '/tracks/@kept/credits' }, { kind: 'removed', path: '/tracks/@kept/previewAssetId' },
      { kind: 'added', path: '/tracks/@new' },
    ])
  })

  it('ignores object key ordering but preserves duplicate reference ordering', () => {
    expect(diffSnapshots({ site: { seo: { title: 'Title', description: 'D' } } }, { site: { seo: { description: 'D', title: 'Title' } } })).toEqual([])
    expect(diffSnapshots({ homepage: { sections: [{ id: 'music', releaseIds: ['a', 'a', 'b'] }] } }, { homepage: { sections: [{ id: 'music', releaseIds: ['a', 'b', 'a'] }] } })[0]!.changes).toEqual([
      { kind: 'reordered', path: '/homepage/sections/@music/releaseIds', beforePath: '/homepage/sections/0/releaseIds', afterPath: '/homepage/sections/0/releaseIds', before: ['a', 'a', 'b'], after: ['a', 'b', 'a'] },
    ])
  })

  it('uses escaped identity paths and exact pointers on both sides', () => {
    expect(diffSnapshots({ assets: [{ id: 'a/b~c', alt: 'A' }] }, { assets: [{ id: 'a/b~c', alt: 'B' }] })[0]!.changes[0]).toMatchObject({
      path: '/assets/@a~1b~0c/alt', beforePath: '/assets/0/alt', afterPath: '/assets/0/alt',
    })
  })

  it('handles unkeyed arrays and structural changes without losing null or missing fields', () => {
    const changes = diffSnapshots({ artist: { name: null, links: [1, 2] } }, { artist: { name: '', links: [1, 3], bio: null } })[0]!.changes
    expect(changes.map(change => [change.path, change.kind, change.before, change.after])).toEqual([
      ['/artist/bio', 'added', undefined, null], ['/artist/links', 'changed', [1, 2], [1, 3]], ['/artist/name', 'changed', null, ''],
    ])
  })

  it('keeps full values in differences but bounds value rendering for large text and collections', () => {
    const text = '<script>alert(1)</script>'.repeat(10000)
    expect(diffSnapshots({ site: { description: 'short' } }, { site: { description: text } })[0]!.changes[0]!.after).toBe(text)
    expect(formatDiffValue(text).length).toBeLessThan(17000)
    expect(formatDiffValue(Array.from({ length: 10000 }, (_, id) => ({ id, text }))).length).toBeLessThan(20000)
    expect(formatDiffValue(undefined)).toBe('—')
  })
})
