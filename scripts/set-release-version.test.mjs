import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import { rm } from 'node:fs/promises'
import test from 'node:test'

const manifests = ['package.json', 'apps/web/package.json', 'apps/admin/package.json', 'packages/schema/package.json']
const goVersion = 'apps/server/cmd/api/version.go'

/** Build an isolated repository so release tests never modify the working tree. */
async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'yujian-version-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  for (const path of [...manifests, goVersion]) {
    await mkdir(dirname(join(root, path)), { recursive: true })
    await writeFile(join(root, path), await readFile(new URL(`../${path}`, import.meta.url)))
  }
  return root
}

test('发布同步全部 Node 包和 Go 版本，保留清单中的非版本字段且重复执行不改变文件', async (t) => {
  const { readReleaseVersion, setReleaseVersion } = await import('./set-release-version.mjs')
  const root = await fixture(t)
  const before = await Promise.all(manifests.map(async path => JSON.parse(await readFile(join(root, path), 'utf8'))))
  setReleaseVersion('1.2.3', root)
  assert.equal(readReleaseVersion(root), '1.2.3')
  for (const [index, path] of manifests.entries()) {
    assert.deepEqual(JSON.parse(await readFile(join(root, path), 'utf8')), { ...before[index], version: '1.2.3' })
  }
  assert.match(await readFile(join(root, goVersion), 'utf8'), /const Version = "1\.2\.3"/)
  const bytes = await Promise.all([...manifests, goVersion].map(path => readFile(join(root, path), 'utf8')))
  setReleaseVersion('1.2.3', root)
  assert.deepEqual(await Promise.all([...manifests, goVersion].map(path => readFile(join(root, path), 'utf8'))), bytes)
})

test('非法目标版本或已有版本不一致时，在任何文件写入前失败', async (t) => {
  const { readReleaseVersion, setReleaseVersion } = await import('./set-release-version.mjs')
  const root = await fixture(t)
  const before = await readFile(join(root, 'package.json'), 'utf8')
  for (const version of ['v1.2.3', '01.2.3', '1.2.3\n', '1.2.3-rc.1', '1.2', '1.2.3"']) {
    assert.throws(() => setReleaseVersion(version, root), /版本/)
  }
  const web = JSON.parse(await readFile(join(root, manifests[1]), 'utf8'))
  web.version = '99.0.0'
  await writeFile(join(root, manifests[1]), JSON.stringify(web))
  assert.throws(() => readReleaseVersion(root), /不一致/)
  assert.throws(() => setReleaseVersion('1.2.3', root), /不一致/)
  assert.equal(await readFile(join(root, 'package.json'), 'utf8'), before)
})

test('Go 版本文件缺失时不会部分修改 Node 清单', async (t) => {
  const { setReleaseVersion } = await import('./set-release-version.mjs')
  const root = await fixture(t)
  const before = await readFile(join(root, 'package.json'), 'utf8')
  await rm(join(root, goVersion))
  assert.throws(() => setReleaseVersion('1.2.3', root), /ENOENT/)
  assert.equal(await readFile(join(root, 'package.json'), 'utf8'), before)
})
