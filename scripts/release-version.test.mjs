import assert from 'node:assert/strict'
import test from 'node:test'

test('首次发布从 0.0.0 递增，minor 得到 0.1.0', async () => {
  const { nextVersion } = await import('./release-version.mjs')
  assert.equal(nextVersion([]), '0.0.1')
  assert.equal(nextVersion([], 'minor'), '0.1.0')
  assert.equal(nextVersion([], 'major'), '1.0.0')
})

test('按数字选择最高稳定标签并清零较低版本位', async () => {
  const { nextVersion } = await import('./release-version.mjs')
  const tags = ['v2.9.9', 'v2.10.3', 'v1.99.99']
  assert.equal(nextVersion(tags, 'patch'), '2.10.4')
  assert.equal(nextVersion(tags, 'minor'), '2.11.0')
  assert.equal(nextVersion(tags, 'major'), '3.0.0')
})

test('预发布、前导零及非版本标签不改变稳定版本', async () => {
  const { nextVersion } = await import('./release-version.mjs')
  assert.equal(nextVersion(['v1.2.3', 'v9.0.0-rc.1', 'v01.0.0', 'archive', '99.0.0', 'v3.0.0\n']), '1.2.4')
})

test('版本递增不受 JavaScript 安全整数范围限制', async () => {
  const { nextVersion } = await import('./release-version.mjs')
  assert.equal(nextVersion(['v1.9007199254740993.4'], 'minor'), '1.9007199254740994.0')
})

test('拒绝非法版本递增选项', async () => {
  const { nextVersion } = await import('./release-version.mjs')
  for (const bump of ['', 'preview', 'PATCH', '__proto__', 'patch\n']) {
    assert.throws(() => nextVersion([], bump), /patch.*minor.*major/)
  }
})
