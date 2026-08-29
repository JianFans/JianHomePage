import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

test('EdgeOne 构建执行完整发布门禁', async () => {
  const config = JSON.parse(await readFile(new URL('../edgeone.json', import.meta.url), 'utf8'))
  assert.equal(config.buildCommand, 'pnpm verify')
})
