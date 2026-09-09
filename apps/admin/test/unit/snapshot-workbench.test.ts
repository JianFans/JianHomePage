import type { YujianContentSnapshot } from '@yujian/schema'
import { describe, expect, it } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import {
  analyzeSnapshotText,
  createSnapshotExport,
  readSnapshotImport,
  SnapshotImportError,
} from '../../utils/snapshot-workbench'

const fixture = fixtureData as unknown as YujianContentSnapshot

describe('快照工作台工具', () => {
  it('分析有效快照并生成内容摘要', () => {
    const result = analyzeSnapshotText(JSON.stringify(fixture))

    expect(result.issues).toHaveLength(0)
    expect(result.snapshot?.releaseId).toBe('rel_fixture_20260829')
    expect(result.summary).toEqual({
      sections: 6,
      releases: 5,
      tracks: 5,
      videos: 3,
      events: 2,
      moments: 3,
      assets: 15,
      previews: 3,
    })
  })

  it('区分 JSON、对象根和内容契约问题', () => {
    expect(analyzeSnapshotText('{').issues).toEqual([{
      path: '/',
      source: 'editor',
      code: 'invalid-json',
    }])
    expect(analyzeSnapshotText('[]').issues).toEqual([{
      path: '/',
      source: 'editor',
      code: 'object-root',
    }])
    expect(analyzeSnapshotText('{}').issues).toContainEqual({
      path: '/schemaVersion',
      source: 'schema',
      code: 'required',
    })
  })

  it('导入大小合规的 JSON 文本', async () => {
    const contents = JSON.stringify(fixture)

    await expect(readSnapshotImport({
      name: 'draft.JSON',
      size: contents.length,
      text: async () => contents,
    })).resolves.toBe(contents)
  })

  it.each([
    ['非 JSON 文件', { name: 'draft.txt', size: 2, text: async () => '{}' }, 'invalid-extension'],
    ['空文件', { name: 'draft.json', size: 0, text: async () => '' }, 'empty-file'],
    ['超过大小限制的文件', { name: 'draft.json', size: 2 * 1024 * 1024 + 1, text: async () => '{}' }, 'file-too-large'],
    ['无法读取的文件', { name: 'draft.json', size: 2, text: async () => { throw new Error('disk') } }, 'read-failed'],
  ])('拒绝%s并返回稳定错误代码', async (_name, file, code) => {
    await expect(readSnapshotImport(file)).rejects.toMatchObject({
      name: SnapshotImportError.name,
      code,
    })
  })

  it('导出可重新校验的格式化快照', () => {
    const exported = createSnapshotExport(fixture)

    expect(exported).toMatchObject({
      filename: 'rel_fixture_20260829.json',
      mimeType: 'application/json',
    })
    expect(exported.contents.endsWith('\n')).toBe(true)
    expect(analyzeSnapshotText(exported.contents).issues).toHaveLength(0)
  })

  it('不把不安全的发布标识写入文件名', () => {
    const unsafe = { ...fixture, releaseId: '../secret' } as YujianContentSnapshot

    expect(createSnapshotExport(unsafe).filename).toBe('yujian-snapshot.json')
  })
})
