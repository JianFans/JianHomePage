import assert from 'node:assert/strict'
import { mkdtemp, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { spawnSync } from 'node:child_process'
import test from 'node:test'
import { fileURLToPath } from 'node:url'

test('EdgeOne 构建执行完整发布门禁', async () => {
  const config = JSON.parse(await readFile(new URL('../edgeone.json', import.meta.url), 'utf8'))
  const packageConfig = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'))
  assert.equal(config.buildCommand, 'pnpm verify:edgeone')
  assert.equal(packageConfig.scripts['verify:edgeone'], 'node scripts/require-content-snapshot.mjs && pnpm verify')
})

test('EdgeOne 构建拒绝缺失或开发 fixture 快照', async () => {
  const script = fileURLToPath(new URL('./require-content-snapshot.mjs', import.meta.url))
  const workspaceRoot = fileURLToPath(new URL('..', import.meta.url))
  for (const value of ['', 'content/fixtures/homepage.json']) {
    const result = spawnSync(process.execPath, [script], {
      cwd: workspaceRoot,
      encoding: 'utf8',
      env: { ...process.env, CONTENT_SNAPSHOT_PATH: value },
    })
    assert.notEqual(result.status, 0)
    assert.match(result.stderr, /CONTENT_SNAPSHOT_PATH/)
  }

  const directory = await mkdtemp(join(tmpdir(), 'yujian-edgeone-'))
  const snapshotPath = join(directory, 'release.json')
  await writeFile(snapshotPath, '{}')
  const result = spawnSync(process.execPath, [script], {
    cwd: workspaceRoot,
    encoding: 'utf8',
    env: { ...process.env, CONTENT_SNAPSHOT_PATH: snapshotPath },
  })
  assert.equal(result.status, 0, result.stderr)
})
