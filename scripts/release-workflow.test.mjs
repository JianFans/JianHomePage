import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { parse } from 'yaml'

const workflow = parse(await readFile(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8'))
const publish = workflow.jobs.publish.steps.find(step => step.run).run
const sha = 'a'.repeat(40)
const bash = process.platform === 'win32'
  ? join(execFileSync('git', ['--exec-path'], { encoding: 'utf8' }).trim(), '../../../bin/bash.exe') : 'bash'

// Simulate only the GitHub CLI boundary; execute the real workflow's Bash unchanged.
const githubStub = `#!/usr/bin/env node
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { basename, join } from 'node:path';
const args = process.argv.slice(2);
const path = process.env.RELEASE_TEST_STATE;
const state = JSON.parse(readFileSync(path, 'utf8'));
const value = flag => args[args.indexOf(flag) + 1];
const save = () => writeFileSync(path, JSON.stringify(state));
if (args[0] === 'api') { console.log(state.sha); process.exit(0); }
if (args[0] !== 'release') process.exit(90);
switch (args[1]) {
  case 'view':
    if (!state.exists) process.exit(1);
    console.log(value('--json') === 'isDraft' ? state.isDraft : Object.keys(state.assets).join('\\n'));
    break;
  case 'create':
    if (state.exists) process.exit(1);
    state.exists = true; state.isDraft = true; state.creates++; save(); break;
  case 'upload': {
    const name = basename(args[3]);
    if (state.assets[name]) process.exit(1);
    state.assets[name] = readFileSync(args[3]).toString('base64'); state.uploads.push(name); save(); break;
  }
  case 'download': {
    const name = value('--pattern');
    if (!state.assets[name]) process.exit(1);
    mkdirSync(value('--dir'), { recursive: true });
    writeFileSync(join(value('--dir'), name), Buffer.from(state.assets[name], 'base64')); break;
  }
  case 'edit':
    state.isDraft = false; state.edits++; save();
    if (state.losePublishResponse) { state.losePublishResponse = false; save(); process.exit(1); }
    break;
  default: process.exit(90);
}
`

/** Supply local archives and an isolated GitHub CLI, without building the application or using the network. */
async function fixture(t, overrides = {}) {
  const directory = await mkdtemp(join(tmpdir(), 'yujian-release-test-'))
  const bashDirectory = process.platform === 'win32'
    ? `/${directory[0].toLowerCase()}${directory.slice(2).replaceAll('\\', '/')}` : directory
  t.after(() => rm(directory, { recursive: true, force: true }))
  const bin = join(directory, 'bin')
  await mkdir(bin)
  await writeFile(join(bin, 'gh.mjs'), githubStub, { mode: 0o755 })
  await writeFile(join(bin, 'gh'), '#!/usr/bin/env bash\nexec node "$(dirname "$0")/gh.mjs" "$@"\n', { mode: 0o755 })
  const assets = {}
  let checksums = ''
  for (const arch of ['amd64', 'arm64']) {
    const name = `yujian-server-0.1.0-linux-${arch}.tar.gz`
    const bytes = Buffer.from(`archive-${arch}`)
    await writeFile(join(directory, name), bytes)
    assets[name] = bytes.toString('base64')
    checksums += `${createHash('sha256').update(bytes).digest('hex')} *${name}\n`
  }
  assets['SHA256SUMS.txt'] = Buffer.from(checksums).toString('base64')
  const statePath = join(directory, 'state.json')
  await writeFile(statePath, JSON.stringify({ sha, exists: true, isDraft: true, assets: {}, creates: 0, uploads: [], edits: 0, ...overrides }))
  return {
    assets,
    state: async () => JSON.parse(await readFile(statePath, 'utf8')),
    run: () => spawnSync(bash, ['--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', `export PATH="${bashDirectory}/bin:$PATH"\n${publish}`], {
      cwd: directory, encoding: 'utf8',
      env: { ...process.env, RELEASE_TEST_STATE: statePath, RELEASE_TAG: 'v0.1.0', RELEASE_VERSION: '0.1.0', RELEASE_SHA: sha, RELEASE_IMAGE: 'ghcr.io/example/site', GH_REPO: 'example/site', RUNNER_TEMP: bashDirectory },
    }),
  }
}

test('首次发布先创建草稿，上传完整文件后才公开', async (t) => {
  const release = await fixture(t, { exists: false })
  const result = release.run()
  assert.equal(result.status, 0, result.stderr)
  const state = await release.state()
  assert.equal(state.creates, 1)
  assert.equal(state.isDraft, false)
  assert.equal(state.edits, 1)
  assert.deepEqual(state.assets, release.assets)
})

test('中断留下的草稿仅补传缺失文件，不覆盖已上传的相同文件', async (t) => {
  const name = 'yujian-server-0.1.0-linux-amd64.tar.gz'
  const release = await fixture(t, { assets: { [name]: Buffer.from('archive-amd64').toString('base64') } })
  const result = release.run()
  assert.equal(result.status, 0, result.stderr)
  const state = await release.state()
  assert.equal(state.creates, 0)
  assert.equal(state.uploads.length, 2)
  assert.ok(!state.uploads.includes(name))
  assert.equal(state.isDraft, false)
})

test('公开响应丢失后重跑只验证现有版本，不再次写入', async (t) => {
  const release = await fixture(t, { exists: false, losePublishResponse: true })
  assert.notEqual(release.run().status, 0)
  const before = await release.state()
  assert.equal(before.isDraft, false)
  const result = release.run()
  assert.equal(result.status, 0, result.stderr)
  assert.deepEqual(await release.state(), before)
})

test('已有同名文件内容不一致时拒绝覆盖和公开', async (t) => {
  const release = await fixture(t, { assets: { 'yujian-server-0.1.0-linux-amd64.tar.gz': Buffer.from('different').toString('base64') } })
  assert.notEqual(release.run().status, 0)
  const state = await release.state()
  assert.equal(state.isDraft, true)
  assert.deepEqual(state.uploads, [])
  assert.equal(state.edits, 0)
})

test('已公开但文件不完整的版本拒绝补写', async (t) => {
  const release = await fixture(t, { isDraft: false })
  assert.notEqual(release.run().status, 0)
  assert.deepEqual((await release.state()).uploads, [])
})

test('标签偏离已验证提交时不创建或修改 Release', async (t) => {
  const release = await fixture(t, { exists: false, sha: 'b'.repeat(40) })
  assert.notEqual(release.run().status, 0)
  const state = await release.state()
  assert.equal(state.creates, 0)
  assert.deepEqual(state.uploads, [])
})
