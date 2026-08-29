import { mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, resolve } from 'node:path'

import { describe, expect, it } from 'vitest'
import {
  loadBuildSnapshot,
  loadContentSnapshot,
  resolveContentSnapshotPath,
} from '../../utils/build-snapshot'

describe('构建时内容快照', () => {
	const fixture = JSON.parse(readFileSync(resolve(process.cwd(), '../../content/fixtures/homepage.json'), 'utf8'))

  it('未配置路径时回退到默认 fixture', () => {
    const workspaceRoot = 'D:/workspace'
    const defaultPath = 'D:/workspace/content/fixtures/homepage.json'

    expect(resolveContentSnapshotPath(undefined, workspaceRoot, defaultPath)).toBe(defaultPath)
    expect(resolveContentSnapshotPath('', workspaceRoot, defaultPath)).toBe(defaultPath)
  })

  it('相对路径按仓库根目录解析并读取快照', () => {
    const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
    const snapshotPath = join(root, 'release.json')
		writeFileSync(snapshotPath, JSON.stringify(fixture))

    const resolved = resolveContentSnapshotPath('release.json', root, join(root, 'fallback.json'))
    expect(resolved).toBe(snapshotPath)
		expect(loadContentSnapshot(resolved)).toEqual(fixture)
    expect(loadBuildSnapshot({
      envPath: 'release.json',
      workspaceRoot: root,
      defaultPath: join(root, 'fallback.json'),
		})).toEqual(fixture)
  })

  it('无效快照会让构建失败并指出文件', () => {
    const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
    const snapshotPath = join(root, 'broken.json')
    writeFileSync(snapshotPath, '{"schemaVersion":')

    expect(() => loadContentSnapshot(snapshotPath)).toThrow(/broken\.json/)
	})

	it('缺少必填字段会让构建失败', () => {
		const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
		const snapshotPath = join(root, 'missing-site.json')
		const invalid = structuredClone(fixture)
		delete invalid.site
		writeFileSync(snapshotPath, JSON.stringify(invalid))

		expect(() => loadContentSnapshot(snapshotPath)).toThrow(/site/)
	})

	it('非法外链会让构建失败', () => {
		const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
		const snapshotPath = join(root, 'unsafe-url.json')
		const invalid = structuredClone(fixture)
		invalid.site.canonicalUrl = 'http://yujian.me'
		writeFileSync(snapshotPath, JSON.stringify(invalid))

		expect(() => loadContentSnapshot(snapshotPath)).toThrow(/canonicalUrl/)
	})

	it('非法日期会让构建失败', () => {
		const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
		const snapshotPath = join(root, 'invalid-date.json')
		const invalid = structuredClone(fixture)
		invalid.generatedAt = 'not-a-date'
		writeFileSync(snapshotPath, JSON.stringify(invalid))

		expect(() => loadContentSnapshot(snapshotPath)).toThrow(/generatedAt/)
	})

	it('悬空内容引用会让构建失败', () => {
		const root = mkdtempSync(join(tmpdir(), 'yujian-snapshot-'))
		const snapshotPath = join(root, 'broken-reference.json')
		const invalid = structuredClone(fixture)
		invalid.releases[0].trackIds[0] = 'track_missing'
		writeFileSync(snapshotPath, JSON.stringify(invalid))

		expect(() => loadContentSnapshot(snapshotPath)).toThrow(/trackIds\/0/)
	})
})
