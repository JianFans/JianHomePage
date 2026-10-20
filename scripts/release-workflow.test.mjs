import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdtemp, mkdir, readFile, rm, writeFile, copyFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { dirname, join } from 'node:path'
import test from 'node:test'
import { parse } from 'yaml'

const workflow = parse(await readFile(new URL('../.github/workflows/release.yml', import.meta.url), 'utf8'))
const preparation = parse(await readFile(new URL('../.github/workflows/release-prepare.yml', import.meta.url), 'utf8'))
const publish = workflow.jobs.publish.steps.find(step => step.run).run
const sha = 'a'.repeat(40)
const bash = process.platform === 'win32'
  ? join(execFileSync('git', ['--exec-path'], { encoding: 'utf8' }).trim(), '../../../bin/bash.exe') : 'bash'

/** Run the actual preparation shell against a local bare remote without credentials or builds. */
async function preparationFixture(t) {
  const directory = await mkdtemp(join(tmpdir(), 'yujian-prepare-test-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const remote = join(directory, 'remote.git')
  const checkout = join(directory, 'checkout')
  const git = (args, cwd = checkout) => execFileSync('git', args, { cwd, encoding: 'utf8', stdio: ['ignore', 'pipe', 'pipe'] }).trim()
  git(['init', '--bare', '--initial-branch=master', remote], directory)
  git(['clone', remote, checkout], directory)
  git(['config', 'user.name', 'Release Test'])
  git(['config', 'user.email', 'release-test@example.invalid'])
  git(['config', 'commit.gpgsign', 'false'])
  git(['config', 'tag.gpgsign', 'false'])
  git(['config', 'core.autocrlf', 'false'])
  const files = ['package.json', 'apps/web/package.json', 'apps/admin/package.json', 'packages/schema/package.json',
    'apps/server/cmd/api/version.go', 'scripts/release-version.mjs', 'scripts/set-release-version.mjs']
  for (const file of files) {
    await mkdir(dirname(join(checkout, file)), { recursive: true })
    await copyFile(new URL(`../${file}`, import.meta.url), join(checkout, file))
  }
  // Keep the simulated first release stable after the real repository is versioned by Actions.
  for (const file of files.filter(file => file.endsWith('package.json'))) {
    const manifest = JSON.parse(await readFile(join(checkout, file), 'utf8'))
    await writeFile(join(checkout, file), JSON.stringify({ ...manifest, version: '0.0.0' }, null, 2) + '\n')
  }
  await writeFile(join(checkout, 'apps/server/cmd/api/version.go'), 'package main\n\nconst Version = "0.0.0"\n')
  git(['add', '.'])
  git(['commit', '-m', 'initial source'])
  git(['push', 'origin', 'HEAD:master'])
  const initial = git(['rev-parse', 'HEAD'])
  const output = join(directory, 'output.txt')
  const script = preparation.jobs.prepare.steps.find(step => step.id === 'version').run
  return {
    git, remote, checkout, initial,
    run: (body = script, overrides = {}) => spawnSync(bash, ['--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', body], {
      cwd: checkout, encoding: 'utf8',
      env: { ...process.env, GITHUB_REF: 'refs/heads/master', DEFAULT_BRANCH: 'master', BUMP: 'minor', GITHUB_REPOSITORY: 'Example/Site', GITHUB_OUTPUT: output, ...overrides },
    }),
    outputs: async () => Object.fromEntries((await readFile(output, 'utf8')).trim().split(/\r?\n/).map(line => line.split('='))),
    script,
  }
}

test('准备阶段同步所有版本文件并将版本提交和标签原子推送到同一 SHA', async (t) => {
  const release = await preparationFixture(t)
  const result = release.run()
  assert.equal(result.status, 0, result.stderr)
  const outputs = await release.outputs()
  assert.equal(outputs.version, '0.1.0')
  assert.equal(outputs.tag, 'v0.1.0')
  assert.equal(outputs.image, 'ghcr.io/example/site')
  assert.notEqual(outputs.sha, release.initial)
  assert.equal(release.git(['rev-parse', 'HEAD^']), release.initial)
  for (const ref of ['refs/heads/master', 'refs/tags/v0.1.0']) {
    assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', ref]), outputs.sha)
  }
  for (const file of ['package.json', 'apps/web/package.json', 'apps/admin/package.json', 'packages/schema/package.json']) {
    assert.equal(JSON.parse(release.git(['show', `${outputs.sha}:${file}`])).version, outputs.version)
  }
  assert.match(release.git(['show', `${outputs.sha}:apps/server/cmd/api/version.go`]), /const Version = "0\.1\.0"/)
  assert.equal(release.git(['status', '--porcelain']), '')
})

test('准备阶段拒绝非默认分支且不写远端引用', async (t) => {
  const release = await preparationFixture(t)
  const result = release.run(undefined, { GITHUB_REF: 'refs/heads/topic' })
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /default branch/)
  assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', 'master']), release.initial)
  assert.equal(release.git(['--git-dir', release.remote, 'tag', '--list']), '')
})

test('准备阶段拒绝过时的启动提交', async (t) => {
  const release = await preparationFixture(t)
  release.git(['commit', '--allow-empty', '-m', 'concurrent change'])
  release.git(['push', 'origin', 'HEAD:master'])
  release.git(['switch', '--detach', release.initial])
  const result = release.run()
  assert.notEqual(result.status, 0)
  assert.match(result.stderr, /Default branch changed/)
  assert.equal(release.git(['--git-dir', release.remote, 'tag', '--list']), '')
})

test('推送时出现标签冲突，默认分支也不被部分更新', async (t) => {
  const release = await preparationFixture(t)
  // Inject a concurrent tag creation after the fetch and before the real atomic push.
  const race = release.script.replace('git push --atomic', 'git push origin "HEAD^:refs/tags/v$version"\ngit push --atomic')
  const result = release.run(race)
  assert.notEqual(result.status, 0)
  assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', 'master']), release.initial)
  assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', 'v0.1.0']), release.initial)
})

test('推送时默认分支已被更新，不留下孤立的版本标签', async (t) => {
  const release = await preparationFixture(t)
  // A sibling commit advances the remote after the latest-branch guard has passed.
  const concurrent = release.git(['-c', 'commit.gpgsign=false', 'commit-tree', 'HEAD^{tree}', '-p', 'HEAD', '-m', 'concurrent change'])
  const race = release.script.replace('git push --atomic', `git push origin "${concurrent}:refs/heads/master"\ngit push --atomic`)
  const result = release.run(race)
  assert.notEqual(result.status, 0)
  assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', 'master']), concurrent)
  assert.equal(release.git(['--git-dir', release.remote, 'tag', '--list']), '')
})

test('发布从事件标签解析提交及文件版本，兼容附注标签', async (t) => {
  const release = await preparationFixture(t)
  assert.equal(release.run().status, 0)
  const outputs = await release.outputs()
  const script = workflow.jobs.prepare.steps.find(step => step.id === 'version').run
  for (const tag of ['v0.1.0', 'annotated']) {
    if (tag === 'annotated') {
      release.git(['tag', '-a', 'v0.1.1', '-m', 'wrong version'])
      // The tag points to valid source but its version must match the files.
      const mismatch = release.run(script, { GITHUB_REF: 'refs/tags/v0.1.1', GITHUB_SHA: outputs.sha })
      assert.notEqual(mismatch.status, 0)
      assert.match(mismatch.stderr, /不一致/)
      release.git(['tag', '-d', 'v0.1.0'])
      release.git(['tag', '-a', 'v0.1.0', '-m', 'release'])
    }
    const result = release.run(script, { GITHUB_REF: 'refs/tags/v0.1.0', GITHUB_SHA: outputs.sha })
    assert.equal(result.status, 0, result.stderr)
    assert.deepEqual(await release.outputs(), outputs)
  }
})

test('发布拒绝分支、非稳定标签和事件 SHA 不匹配，不写远端', async (t) => {
  const release = await preparationFixture(t)
  assert.equal(release.run().status, 0)
  const outputs = await release.outputs()
  const script = workflow.jobs.prepare.steps.find(step => step.id === 'version').run
  for (const ref of ['refs/heads/master', 'refs/tags/v01.0.0', 'refs/tags/v0.1.0-rc.1', 'refs/tags/v0.1.0']) {
    const result = release.run(script, { GITHUB_REF: ref, GITHUB_SHA: release.initial })
    assert.notEqual(result.status, 0, ref)
  }
  assert.equal(release.git(['--git-dir', release.remote, 'rev-parse', 'master']), outputs.sha)
})

// Simulate only the GitHub CLI boundary; execute the real workflow's Bash unchanged.
const githubStub = `#!/usr/bin/env node
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { basename, join } from 'node:path';
const args = process.argv.slice(2);
const path = process.env.RELEASE_TEST_STATE;
const state = JSON.parse(readFileSync(path, 'utf8'));
const value = flag => args[args.indexOf(flag) + 1];
const save = () => writeFileSync(path, JSON.stringify(state));
if (args[0] === 'workflow') {
  state.dispatches = [...(state.dispatches || []), args]; save();
  process.exit(state.failDispatch ? 1 : 0);
}
if (args[0] === 'api') {
  if (args[1] !== 'repos/example/site/commits/refs/tags/v0.1.0' || value('--jq') !== '.sha') process.exit(90);
  console.log(state.sha); process.exit(0);
}
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

/** Supply raw executable bytes and an isolated GitHub CLI without builds or network access. */
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
    const name = `yujian-server-0.1.0-linux-${arch}`
    const bytes = Buffer.from(`binary-${arch}`)
    await writeFile(join(directory, name), bytes)
    assets[name] = bytes.toString('base64')
    checksums += `${createHash('sha256').update(bytes).digest('hex')} *${name}\n`
  }
  assets['SHA256SUMS.txt'] = Buffer.from(checksums).toString('base64')
  const statePath = join(directory, 'state.json')
  await writeFile(statePath, JSON.stringify({ sha, exists: true, isDraft: true, assets: {}, creates: 0, uploads: [], edits: 0, ...overrides }))
  return {
    assets,
    directory,
    state: async () => JSON.parse(await readFile(statePath, 'utf8')),
    run: (body = publish) => spawnSync(bash, ['--noprofile', '--norc', '-e', '-o', 'pipefail', '-c', `export PATH="${bashDirectory}/bin:$PATH"\n${body}`], {
      cwd: directory, encoding: 'utf8',
      env: { ...process.env, RELEASE_TEST_STATE: statePath, RELEASE_TAG: 'v0.1.0', RELEASE_VERSION: '0.1.0', RELEASE_SHA: sha, RELEASE_IMAGE: 'ghcr.io/example/site', GH_REPO: 'example/site', RUNNER_TEMP: bashDirectory, GITHUB_STEP_SUMMARY: join(directory, 'summary.md') },
    }),
  }
}

test('dispatch 使用准备好的标签启动独立运行，失败时可仅重跑触发步骤', async (t) => {
  const release = await fixture(t, { failDispatch: true })
  const script = preparation.jobs.dispatch.steps[0].run
  assert.notEqual(release.run(script).status, 0)
  const state = await release.state()
  assert.deepEqual(state.dispatches, [['workflow', 'run', 'release.yml', '--ref', 'v0.1.0', '--repo', 'example/site']])
  state.failDispatch = false
  await writeFile(join(release.directory, 'state.json'), JSON.stringify(state))
  const result = release.run(script)
  assert.equal(result.status, 0, result.stderr)
  const after = await release.state()
  assert.deepEqual(after.dispatches[1], state.dispatches[0])
  assert.equal(after.creates, 0)
  assert.deepEqual(after.uploads, [])
  assert.match(await readFile(join(release.directory, 'summary.md'), 'utf8'), /v0\.1\.0/)
})

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

test('缺少任一架构的二进制时，不创建 Release 或上传文件', async (t) => {
  const release = await fixture(t, { exists: false })
  await rm(join(release.directory, 'yujian-server-0.1.0-linux-arm64'))
  assert.notEqual(release.run().status, 0)
  const state = await release.state()
  assert.equal(state.creates, 0)
  assert.deepEqual(state.uploads, [])
})

test('发布只上传指定版本双架构二进制和校验和', async (t) => {
  const release = await fixture(t, { exists: false })
  await writeFile(join(release.directory, 'yujian-server-old-linux-amd64'), 'unrelated file')
  const result = release.run()
  assert.equal(result.status, 0, result.stderr)
  assert.deepEqual((await release.state()).assets, release.assets)
})

test('中断留下的草稿仅补传缺失文件，不覆盖已上传的相同文件', async (t) => {
  const name = 'yujian-server-0.1.0-linux-amd64'
  const release = await fixture(t, { assets: { [name]: Buffer.from('binary-amd64').toString('base64') } })
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
  const release = await fixture(t, { assets: { 'yujian-server-0.1.0-linux-amd64': Buffer.from('different').toString('base64') } })
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
