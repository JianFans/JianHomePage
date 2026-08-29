import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

import { describe, expect, it } from 'vitest'
import {
  loadBuildSnapshot,
  loadContentSnapshot,
  resolveContentSnapshotPath,
} from '../../utils/build-snapshot'

describe('构建时内容快照', () => {
  it('未配置路径时回退到默认 fixture', () => {
    const workspaceRoot = 'D:/workspace'
    const defaultPath = 'D:/workspace/content/fixtures/homepage.json'

    expect(resolveContentSnapshotPath(undefined, workspaceRoot, defaultPath)).toBe(defaultPath)
    expect(resolveContentSnapshotPath('', workspaceRoot, defaultPath)).toBe(defaultPath)
  })

  it('相对路径按仓库根目录解析并读取快照', () => {
    const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
    const snapshotPath = join(root, 'release.json')
    writeFileSync(snapshotPath, JSON.stringify({ schemaVersion: '1.0.0' }))

    const resolved = resolveContentSnapshotPath('release.json', root, join(root, 'fallback.json'))
    expect(resolved).toBe(snapshotPath)
    expect(loadContentSnapshot(resolved)).toEqual({ schemaVersion: '1.0.0' })
    expect(loadBuildSnapshot({
      envPath: 'release.json',
      workspaceRoot: root,
      defaultPath: join(root, 'fallback.json'),
    })).toEqual({ schemaVersion: '1.0.0' })
  })

  it('无效快照会让构建失败并指出文件', () => {
    const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
    const snapshotPath = join(root, 'broken.json')
    writeFileSync(snapshotPath, '{"schemaVersion":')

    expect(() => loadContentSnapshot(snapshotPath)).toThrow(/broken\.json/)
  })
})
