import assert from 'node:assert/strict'
import { mkdtemp, mkdir, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { afterEach, test } from 'node:test'
import {
  DEFAULT_INITIAL_SCRIPT_BUDGET,
  verifyStaticOutput,
} from './verify-static-output.mjs'

const temporaryDirectories = []

afterEach(async () => {
  await Promise.all(temporaryDirectories.splice(0).map(directory => (
    rm(directory, { force: true, recursive: true })
  )))
})

async function createOutput({
  html = '<!doctype html><script type="module" src="/_nuxt/app.js"></script>',
  script = 'console.log("static")',
  robots = 'User-agent: *\nAllow: /',
  sitemap = '<urlset><url><loc>https://yujian.me/</loc></url></urlset>',
} = {}) {
  const directory = await mkdtemp(join(tmpdir(), 'yujian-static-'))
  temporaryDirectories.push(directory)
  await mkdir(join(directory, '_nuxt'), { recursive: true })
  await writeFile(join(directory, 'index.html'), html)
  await writeFile(join(directory, '_nuxt', 'app.js'), script)
  await writeFile(join(directory, 'robots.txt'), robots)
  await writeFile(join(directory, 'sitemap.xml'), sitemap)
  return directory
}

test('拒绝缺少 index.html 的产物', async () => {
  const directory = await mkdtemp(join(tmpdir(), 'yujian-static-'))
  temporaryDirectories.push(directory)

  await assert.rejects(
    verifyStaticOutput(directory),
    /index\.html/,
  )
})

test('拒绝首页引用不存在的本地静态资源', async () => {
  const directory = await createOutput({
    html: '<!doctype html><script type="module" src="/_nuxt/missing.js"></script>',
  })

  await assert.rejects(
    verifyStaticOutput(directory),
    /_nuxt\/missing\.js/,
  )
})

test('拒绝静态产物中的运行时内容 API 标记', async () => {
  const directory = await createOutput({
    script: 'fetch("/api/content/homepage")',
  })

  await assert.rejects(
    verifyStaticOutput(directory),
    /运行时内容 API.*_nuxt\/app\.js/,
  )
})

test('拒绝超过首屏脚本预算的产物', async () => {
  const directory = await createOutput({
    script: 'x'.repeat(DEFAULT_INITIAL_SCRIPT_BUDGET + 1),
  })

  await assert.rejects(
    verifyStaticOutput(directory),
    /首屏 JavaScript.*320 KiB/,
  )
})

test('接受包含发布必需文件且预算内的纯静态产物', async () => {
  const directory = await createOutput()

  const result = await verifyStaticOutput(directory)

  assert.equal(result.initialScriptBytes, 21)
  assert.equal(result.checkedReferences, 1)
})
