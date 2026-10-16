import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import childProcess from 'node:child_process'
import { EventEmitter } from 'node:events'
import filesystem from 'node:fs/promises'
import { access, mkdir, mkdtemp, readFile, readdir, rm, symlink, writeFile } from 'node:fs/promises'
import { createRequire, syncBuiltinESMExports } from 'node:module'
import { tmpdir } from 'node:os'
import { dirname, isAbsolute, join, resolve } from 'node:path'
import { performance } from 'node:perf_hooks'
import test from 'node:test'
import { createBuilder } from '../src/builder.mjs'
import { runBuildCommand } from '../src/subprocess.mjs'

/** Create real isolated source/output trees; replace only the external process boundary. */
async function setup(t, snapshot = { releaseId: 'rel_reviewed', assets: [] }) {
  const root = await mkdtemp(join(tmpdir(), 'publisher-builder-'))
  t.after(() => rm(root, { recursive: true, force: true }))
  const sourceRoot = join(root, 'source')
  const workRoot = join(root, 'work')
  const directory = join(sourceRoot, 'apps/web/.output/public')
  await mkdir(directory, { recursive: true })
  await mkdir(join(sourceRoot, 'apps/web/.nuxt'), { recursive: true })
  await writeFile(join(directory, 'stale.html'), 'old release')
  await writeFile(join(sourceRoot, 'apps/web/.nuxt/stale'), 'old cache')
  const bytes = Buffer.from(JSON.stringify(snapshot))
  const snapshotChecksum = `sha256:${createHash('sha256').update(bytes).digest('hex')}`
  const job = {
    id: `build_${'a'.repeat(64)}`, releaseId: 'rel_reviewed', snapshotChecksum,
    snapshotKey: `snapshots/rel_reviewed/${snapshotChecksum}.json`,
  }
  const calls = []
  const run = async (options) => {
    calls.push(options)
    assert.equal(options.command, process.execPath)
    assert.equal(options.cwd, sourceRoot)
    assert.ok(isAbsolute(options.env.CONTENT_SNAPSHOT_PATH))
    assert.deepEqual(await readFile(options.env.CONTENT_SNAPSHOT_PATH), bytes)
    if (calls.length === 1) {
      await assert.rejects(access(join(directory, 'stale.html')))
      await assert.rejects(access(join(sourceRoot, 'apps/web/.nuxt/stale')))
    }
    if (options.args.includes('@yujian/web')) {
      await mkdir(join(directory, 'media'), { recursive: true })
      await writeFile(join(directory, 'index.html'), '<html>reviewed release</html>')
      await writeFile(join(directory, 'media/hero-studio.webp'), 'fixture')
      await writeFile(join(directory, 'media/approved.webp'), 'reviewed')
    }
  }
  const options = {
    sourceRoot, workRoot, run,
    readSnapshot: async function* () { yield bytes.subarray(0, 5); yield bytes.subarray(5) },
  }
  return { options, job, calls, directory, bytes, root }
}

test('builds reviewed bytes in fixed order, clears stale output and cleans the snapshot', async (t) => {
  const fixture = await setup(t)
  const result = await createBuilder(fixture.options)(fixture.job)
  assert.equal(result, fixture.directory)
  assert.equal(fixture.calls.length, 4)
  for (const call of fixture.calls.filter(call => call.args.includes('--filter'))) {
    assert.ok(call.args.includes('--config.verify-deps-before-run=false'))
  }
  assert.deepEqual(fixture.calls.map(call => call.args.slice(-3)), [
    ['scripts/require-content-snapshot.mjs'],
    ['--filter', '@yujian/schema', 'generate'],
    ['--filter', '@yujian/web', 'generate'],
    ['scripts/verify-static-output.mjs', 'apps/web/.output/public'],
  ])
  assert.match(await readFile(join(result, 'index.html'), 'utf8'), /reviewed/)
  await assert.rejects(access(join(result, 'media/hero-studio.webp')))
  assert.equal(await readFile(join(result, 'media/approved.webp'), 'utf8'), 'reviewed')
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('uses the workspace-installed pnpm CLI without any global Node or Corepack installation', async (t) => {
  const fixture = await setup(t)
  const originalExecPath = process.execPath
  process.execPath = join(fixture.root, 'nonexistent-node-runtime')
  t.after(() => { process.execPath = originalExecPath })
  const directory = await createBuilder(fixture.options)(fixture.job)
  assert.equal(directory, fixture.directory)
  const cli = fixture.calls.find(call => call.args.includes('@yujian/schema')).args[0]
  assert.ok(isAbsolute(cli))
  const installed = createRequire(import.meta.url)
  assert.equal(cli, resolve(dirname(installed.resolve('pnpm/package.json')), 'bin/pnpm.mjs'))
  await access(cli)
})

test('runs the pinned pnpm Node entry offline without global shims or install scripts', async (t) => {
  const fixture = await setup(t)
  await createBuilder(fixture.options)(fixture.job)
  const call = fixture.calls.find(call => call.args.includes('@yujian/schema'))
  const installed = createRequire(import.meta.url)
  const result = childProcess.spawnSync(call.command, [call.args[0], '--version'], {
    cwd: fixture.options.sourceRoot,
    env: { ...call.env, PATH: '', COREPACK_ENABLE_NETWORK: '0' },
    encoding: 'utf8',
    timeout: 15000,
    windowsHide: true,
  })
  assert.equal(result.status, 0, result.stderr)
  assert.equal(result.stdout.trim(), installed('pnpm/package.json').version)
})

test('child environments exclude credentials, NODE_OPTIONS and arbitrary inherited values', async (t) => {
  const fixture = await setup(t)
  const names = ['DATABASE_URL', 'AWS_SECRET_ACCESS_KEY', 'PAGES_API_TOKEN', 'NODE_OPTIONS', 'NUXT_PUBLIC_SECRET']
  const previous = Object.fromEntries(names.map(name => [name, process.env[name]]))
  for (const name of names) process.env[name] = 'private-do-not-copy'
  t.after(() => {
    for (const name of names) {
      if (previous[name] === undefined) delete process.env[name]
      else process.env[name] = previous[name]
    }
  })
  await createBuilder(fixture.options)(fixture.job)
  for (const { env } of fixture.calls) {
    for (const name of names) assert.equal(env[name], undefined)
    assert.equal(env.NODE_ENV, 'production')
    assert.equal(env.CI, 'true')
    assert.equal(env.NUXT_TELEMETRY_DISABLED, '1')
    assert.equal(env.COREPACK_ENABLE_NETWORK, '0')
  }
})

for (const [name, change] of [
  ['checksum mismatch', job => ({ ...job, snapshotChecksum: `sha256:${'0'.repeat(64)}`, snapshotKey: `snapshots/rel_reviewed/sha256:${'0'.repeat(64)}.json` })],
  ['key mismatch', job => ({ ...job, snapshotKey: 'snapshots/rel_other/file.json' })],
  ['release traversal', job => ({ ...job, releaseId: 'rel_../../escape' })],
  ['job traversal', job => ({ ...job, id: '../escape' })],
  ['invalid checksum', job => ({ ...job, snapshotChecksum: 'sha256:bad' })],
]) {
  test(`rejects ${name} before executing build commands`, async (t) => {
    const fixture = await setup(t)
    await assert.rejects(createBuilder(fixture.options)(change(fixture.job)), /Build failed/)
    assert.equal(fixture.calls.length, 0)
  })
}

test('rejects a snapshot whose embedded release differs from the job', async (t) => {
  const fixture = await setup(t, { releaseId: 'rel_other' })
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(fixture.calls.length, 0)
})

test('bounds streamed snapshot bytes and closes an oversized stream', async (t) => {
  const fixture = await setup(t)
  let closed = false
  fixture.options.readSnapshot = async function* () {
    try { yield Buffer.alloc(8 * 1024 * 1024); yield Buffer.from('overflow') } finally { closed = true }
  }
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.ok(closed)
  assert.equal(fixture.calls.length, 0)
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('rejects reviewed snapshots that still reference bundled fixture media', async (t) => {
  const fixture = await setup(t, { releaseId: 'rel_reviewed', assets: [{ src: '/media/hero-studio.webp' }] })
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(fixture.calls.length, 0)
})

test('refuses output templates that retain local fixture references before deleting media', async (t) => {
  const fixture = await setup(t)
  const original = fixture.options.run
  fixture.options.run = async (options) => {
    await original(options)
    if (options.args.includes('@yujian/web')) {
      await writeFile(join(fixture.directory, 'index.html'), '<img src="/media/hero-studio.webp">')
    }
  }
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(fixture.calls.length, 3)
  assert.equal(await readFile(join(fixture.directory, 'media/hero-studio.webp'), 'utf8'), 'fixture')
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('retains legitimate HTTPS media with the same basename as local fixtures', async (t) => {
  const url = 'https://media.example.test/media/hero-studio.webp'
  const fixture = await setup(t, { releaseId: 'rel_reviewed', assets: [{ src: url }] })
  const original = fixture.options.run
  fixture.options.run = async (options) => {
    await original(options)
    if (options.args.includes('@yujian/web')) await writeFile(join(fixture.directory, 'index.html'), `<img src="${url}">`)
  }
  const directory = await createBuilder(fixture.options)(fixture.job)
  assert.match(await readFile(join(directory, 'index.html'), 'utf8'), /https:\/\/media.example.test/)
})

test('permits reviewed local media distinct from the procedural fixture manifest', async (t) => {
  const fixture = await setup(t, { releaseId: 'rel_reviewed', assets: [{ src: '/media/approved.webp' }] })
  const directory = await createBuilder(fixture.options)(fixture.job)
  assert.equal(await readFile(join(directory, 'media/approved.webp'), 'utf8'), 'reviewed')
})

test('stops on process failure, redacts its output and removes temporary snapshots', async (t) => {
  const fixture = await setup(t)
  let executions = 0
  fixture.options.run = async () => { executions++; throw new Error('provider-token-and-build-output') }
  await assert.rejects(createBuilder(fixture.options)(fixture.job), error => error.message === 'Build failed')
  assert.equal(executions, 1)
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('requires a newly built index even when a runner reports success', async (t) => {
  const fixture = await setup(t)
  fixture.options.run = async () => {}
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
})

test('rejects a work directory inside the trusted source', async (t) => {
  const fixture = await setup(t)
  fixture.options.workRoot = join(fixture.options.sourceRoot, 'work')
  await assert.rejects(async () => createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(fixture.calls.length, 0)
})

test('rejects linked output paths without deleting files outside the trusted source', async (t) => {
  const fixture = await setup(t)
  const external = join(fixture.root, 'external')
  await mkdir(external)
  await writeFile(join(external, 'sentinel'), 'preserve')
  const output = join(fixture.options.sourceRoot, 'apps/web/.output')
  await rm(output, { recursive: true })
  await symlink(external, output, process.platform === 'win32' ? 'junction' : 'dir')
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(await readFile(join(external, 'sentinel'), 'utf8'), 'preserve')
  assert.equal(fixture.calls.length, 0)
})

test('rejects linked work directories resolving into the source tree', async (t) => {
  const fixture = await setup(t)
  const actual = join(fixture.options.sourceRoot, 'hidden-work')
  await mkdir(actual)
  await symlink(actual, fixture.options.workRoot, process.platform === 'win32' ? 'junction' : 'dir')
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.deepEqual(await readdir(actual), [])
  assert.equal(fixture.calls.length, 0)
})

test('rejects non-binary snapshot streams and cleans their workspace', async (t) => {
  const fixture = await setup(t)
  fixture.options.readSnapshot = async function* () { yield 'text is not the raw binary contract' }
  await assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/)
  assert.equal(fixture.calls.length, 0)
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('serializes separate builders sharing the process-wide output lock', async (t) => {
  const first = await setup(t)
  const second = await setup(t)
  let release
  let entered
  const enteredPromise = new Promise(resolve => { entered = resolve })
  const blocked = new Promise(resolve => { release = resolve })
  const original = first.options.run
  first.options.run = async (options) => {
    if (first.calls.length === 0) { entered(); await blocked }
    await original(options)
  }
  const one = createBuilder(first.options)(first.job)
  await enteredPromise
  const two = createBuilder(second.options)(second.job)
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(second.calls.length, 0)
  release()
  await Promise.all([one, two])
  assert.equal(second.calls.length, 4)
})

/** A generous watchdog detects deadlock without asserting scheduler-sensitive elapsed times. */
async function bounded(operation) {
  let timer
  try {
    return await Promise.race([
      operation,
      new Promise((resolve, reject) => { timer = setTimeout(() => reject(new Error('Build remained blocked')), 10_000) }),
    ])
  } finally { clearTimeout(timer) }
}

test('overall deadline aborts a pending readSnapshot and releases the queue for the next build', async (t) => {
  const first = await setup(t)
  const next = await setup(t)
  let signal
  first.options.timeoutMs = 1000
  first.options.readSnapshot = (key, options) => { signal = options?.signal; return new Promise(() => {}) }
  const failed = assert.rejects(createBuilder(first.options)(first.job), /Build failed/)
  const success = createBuilder(next.options)(next.job)
  await bounded(Promise.all([failed, success]))
  assert.ok(signal?.aborted)
  assert.equal(first.calls.length, 0)
  assert.equal(next.calls.length, 4)
  assert.deepEqual(await readdir(first.options.workRoot), [])
})

test('overall deadline cancels a stalled stream without awaiting its blocked return', async (t) => {
  const fixture = await setup(t)
  let destroyCalls = 0
  let returnCalls = 0
  fixture.options.timeoutMs = 1000
  fixture.options.readSnapshot = () => ({
    destroy() { destroyCalls++ },
    [Symbol.asyncIterator]() {
      return {
        next: () => new Promise(() => {}),
        return() { returnCalls++; return new Promise(() => {}) },
      }
    },
  })
  await bounded(assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/))
  assert.equal(destroyCalls, 1)
  assert.equal(returnCalls, 1)
  assert.equal(fixture.calls.length, 0)
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('destroys a stream returned after the readSnapshot deadline instead of consuming it', async (t) => {
  const fixture = await setup(t)
  let resolveRead
  let destroyed = false
  fixture.options.timeoutMs = 1000
  fixture.options.readSnapshot = () => new Promise(resolve => { resolveRead = resolve })
  await bounded(assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/))
  resolveRead({ destroy() { destroyed = true }, [Symbol.asyncIterator]() { throw new Error('Must not consume late stream') } })
  await new Promise(resolve => setImmediate(resolve))
  assert.ok(destroyed)
  assert.equal(fixture.calls.length, 0)
})

test('passes remaining overall budget to each command and stops after budget exhaustion', async (t) => {
  const fixture = await setup(t)
  let now = 0
  t.mock.method(performance, 'now', () => now)
  fixture.options.timeoutMs = 10_000
  const budgets = []
  fixture.options.run = async ({ timeoutMs }) => {
    budgets.push(timeoutMs)
    // The first boundary spends part of the budget; the second consumes its remainder.
    now += budgets.length === 1 ? 3000 : timeoutMs + 1
  }
  await bounded(assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/))
  assert.equal(budgets.length, 2)
  assert.ok(budgets[0] <= fixture.options.timeoutMs)
  assert.ok(budgets[1] < budgets[0])
  assert.deepEqual(await readdir(fixture.options.workRoot), [])
})

test('waits for a timed-out runner to terminate before starting the next shared build', async (t) => {
  const first = await setup(t)
  const next = await setup(t)
  first.options.timeoutMs = 1000
  let terminated = false
  first.options.run = async ({ timeoutMs }) => {
    await new Promise(resolve => setTimeout(resolve, timeoutMs + 60))
    terminated = true
    throw new Error('process tree terminated')
  }
  const original = next.options.run
  next.options.run = async (options) => { assert.ok(terminated); await original(options) }
  await bounded(Promise.all([
    assert.rejects(createBuilder(first.options)(first.job), /Build failed/),
    createBuilder(next.options)(next.job),
  ]))
})

for (const method of ['realpath', 'lstat', 'stat', 'readdir', 'readFile']) {
  test(`overall deadline bounds a stalled filesystem ${method} read`, async (t) => {
    const fixture = await setup(t)
    const original = filesystem[method]
    let now = 0
    t.mock.method(performance, 'now', () => now)
    let stalled = false
    let signal
    const mock = t.mock.method(filesystem, method, (...args) => {
      // Scope readFile interception to generated output, preserving the runner snapshot check.
      if (method !== 'readFile' || String(args[0]).endsWith('index.html')) {
        stalled = true
        signal = args[1]?.signal
        now = 10_001
        return new Promise(() => {})
      }
      return original(...args)
    })
    syncBuiltinESMExports()
    t.after(() => { mock.mock.restore(); syncBuiltinESMExports() })
    fixture.options.timeoutMs = 10_000
    await bounded(assert.rejects(createBuilder(fixture.options)(fixture.job), /Build failed/))
    assert.ok(stalled)
    if (method === 'readFile') assert.ok(signal?.aborted)
  })
}

test('deadline waits for an in-flight output deletion before releasing the shared queue', async (t) => {
  const first = await setup(t)
  const next = await setup(t)
  const original = filesystem.rm
  let now = 0
  t.mock.method(performance, 'now', () => now)
  let deleted = false
  let entered
  let release
  const deleting = new Promise(resolve => { entered = resolve })
  const gate = new Promise(resolve => { release = resolve })
  const mock = t.mock.method(filesystem, 'rm', async (...args) => {
    if (String(args[0]) === join(first.options.sourceRoot, 'apps/web/.output')) {
      now = 10_001
      entered()
      await gate
      await original(...args)
      deleted = true
      return
    }
    return original(...args)
  })
  syncBuiltinESMExports()
  t.after(() => { mock.mock.restore(); syncBuiltinESMExports() })
  first.options.timeoutMs = 10_000
  const originalRun = next.options.run
  next.options.run = async options => { assert.ok(deleted); await originalRun(options) }
  const failed = assert.rejects(createBuilder(first.options)(first.job), /Build failed/)
  await bounded(deleting)
  const success = createBuilder(next.options)(next.job)
  await new Promise(resolve => setImmediate(resolve))
  assert.equal(next.calls.length, 0)
  release()
  await bounded(Promise.all([failed, success]))
  assert.equal(first.calls.length, 0)
})

test('nonzero exit kills surviving POSIX descendants before returning failure', { skip: process.platform === 'win32' }, async (t) => {
  const directory = await mkdtemp(join(tmpdir(), 'publisher-descendant-'))
  t.after(() => rm(directory, { recursive: true, force: true }))
  const marker = join(directory, 'late-write')
  const descendant = 'setTimeout(() => { require("node:fs").writeFileSync(process.argv[1], "stale") }, 400)'
  const script = `require('node:child_process').spawn(process.execPath, ['-e', ${JSON.stringify(descendant)}, process.argv[1]], {stdio:'ignore'}); process.exit(2)`
  await assert.rejects(runBuildCommand({ command: process.execPath, args: ['-e', script, marker], cwd: directory, env: {}, timeoutMs: 5000 }), /Build process failed/)
  await new Promise(resolve => setTimeout(resolve, 700))
  await assert.rejects(access(marker))
})

test('Windows abnormal exits await taskkill and fall back if tree termination fails', { skip: process.platform !== 'win32' }, async (t) => {
  let killed = false
  let treeTerminationFinished = false
  let spawns = 0
  const mock = t.mock.method(childProcess, 'spawn', (command, args) => {
    spawns++
    const process = new EventEmitter()
    process.stdout = new EventEmitter()
    process.stderr = new EventEmitter()
    process.pid = 12345
    process.kill = () => { killed = true }
    if (spawns === 1) setImmediate(() => process.emit('close', 2))
    else {
      assert.match(command, /taskkill\.exe$/i)
      assert.deepEqual(args, ['/pid', '12345', '/T', '/F'])
      setImmediate(() => { treeTerminationFinished = true; process.emit('close', 1) })
    }
    return process
  })
  syncBuiltinESMExports()
  t.after(() => { mock.mock.restore(); syncBuiltinESMExports() })
  await assert.rejects(runBuildCommand({ command: process.execPath, args: [], cwd: tmpdir(), env: { SystemRoot: 'C:/Windows' }, timeoutMs: 5000 }), /Build process failed/)
  assert.ok(treeTerminationFinished)
  assert.ok(killed)
})

/** Exercise real subprocess failure modes without exposing their output to callers. */
for (const [name, script, timeoutMs] of [
  ['nonzero exit', 'process.stderr.write("secret"); process.exit(2)', 5000],
  ['timeout', 'setInterval(() => {}, 1000)', 30],
  ['excessive output', 'process.stdout.write(Buffer.alloc(2 * 1024 * 1024))', 5000],
]) {
  test(`process runner rejects ${name} with a generic error`, async () => {
    await assert.rejects(runBuildCommand({
      command: process.execPath, args: ['-e', script], cwd: tmpdir(), env: {}, timeoutMs,
    }), error => error.message === 'Build process failed')
  })
}

test('process runner accepts a successful silent process', async () => {
  await runBuildCommand({ command: process.execPath, args: ['-e', 'process.exit(0)'], cwd: tmpdir(), env: {}, timeoutMs: 5000 })
})

test('process runner sanitizes spawn errors', async () => {
  await assert.rejects(runBuildCommand({ command: join(tmpdir(), 'missing-publisher-command'), args: [], cwd: tmpdir(), env: {}, timeoutMs: 5000 }), error => error.message === 'Build process failed')
})
