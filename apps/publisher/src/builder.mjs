import { createHash } from 'node:crypto'
import { lstat, mkdir, mkdtemp, readFile, readdir, realpath, rm, stat, writeFile } from 'node:fs/promises'
import { createRequire } from 'node:module'
import { dirname, extname, isAbsolute, join, relative, resolve } from 'node:path'
import { performance } from 'node:perf_hooks'
import { runBuildCommand } from './subprocess.mjs'

const MAX_SNAPSHOT_BYTES = 8 * 1024 * 1024
const fixtureNames = [
  'hero-studio.webp', 'hero-stage.webp', 'preview-sample.wav',
  ...Array.from({ length: 5 }, (_, i) => `cover-0${i + 1}.webp`),
  ...Array.from({ length: 3 }, (_, i) => `video-0${i + 1}.webp`),
  ...Array.from({ length: 3 }, (_, i) => `moment-0${i + 1}.webp`),
]
const fixturePaths = new Set(fixtureNames.map(name => `/media/${name}`))
const localFixtureReference = new RegExp(`(?:^|["'\\s(=])/media/(?:${fixtureNames.map(name => name.replaceAll('.', '\\.')).join('|')})(?:$|[?"'#\\s)<>,])`)
let buildQueue = Promise.resolve()

/** Check containment using path components, including cross-drive Windows paths. */
function inside(root, target) {
  const path = relative(root, target)
  return path === '' || (path !== '..' && !path.startsWith(`..${process.platform === 'win32' ? '\\' : '/'}`) && !isAbsolute(path))
}

/** Use the publisher's fixed installed CLI; global shims and Corepack may download code. */
function pnpmCli() {
  // pnpm 11 exports its package manifest; its executable subpath is intentionally private.
  const installed = createRequire(import.meta.url)
  return resolve(dirname(installed.resolve('pnpm')), installed('pnpm').bin.pnpm)
}

/** Whitelist OS process essentials; build configuration cannot carry publisher secrets. */
function buildEnvironment(snapshotPath) {
  const env = {}
  for (const [key, value] of Object.entries(process.env)) {
    if (['PATH', 'SYSTEMROOT', 'WINDIR', 'TEMP', 'TMP', 'TMPDIR'].includes(key.toUpperCase())) env[key] = value
  }
  return { ...env, NODE_ENV: 'production', CI: 'true', NUXT_TELEMETRY_DISABLED: '1', CONTENT_SNAPSHOT_PATH: snapshotPath }
}

/** Reject symlinked deletion targets and ancestors before touching fixed build directories. */
async function fixedDirectory(sourceRoot, segments, budget) {
  let current = sourceRoot
  for (const segment of segments) {
    budget.remaining()
    current = join(current, segment)
    try {
      if ((await budget.wait(lstat(current))).isSymbolicLink()) throw new Error('Unsafe build path')
    } catch (error) {
      if (error.code !== 'ENOENT') throw error
    }
  }
  if (!inside(sourceRoot, current) || current === sourceRoot) throw new Error('Unsafe build path')
  return current
}

/** One monotonic budget spans snapshot acquisition, iteration, and every build command. */
function buildBudget(timeoutMs) {
  const controller = new AbortController()
  const deadline = performance.now() + timeoutMs
  const timer = setTimeout(() => controller.abort(), timeoutMs)
  return {
    signal: controller.signal,
    /** Check elapsed monotonic time even when synchronous work delays the abort timer. */
    remaining() {
      const remaining = Math.ceil(deadline - performance.now())
      if (remaining <= 0) controller.abort()
      if (controller.signal.aborted) throw new Error('Build deadline exceeded')
      return remaining
    },
    /** Bound read-only external waits; mutations must finish before releasing the build lock. */
    wait(operation) {
      return new Promise((resolve, reject) => {
        const onAbort = () => reject(new Error('Build deadline exceeded'))
        controller.signal.addEventListener('abort', onAbort, { once: true })
        Promise.resolve(operation).then((value) => {
          controller.signal.removeEventListener('abort', onAbort)
          if (controller.signal.aborted) onAbort()
          else resolve(value)
        }, (error) => {
          controller.signal.removeEventListener('abort', onAbort)
          reject(error)
        })
        try { this.remaining() } catch (error) { reject(error) }
        if (controller.signal.aborted) onAbort()
      })
    },
    /** Remove the deadline timer once construction or cleanup takes ownership of completion. */
    dispose() { clearTimeout(timer) },
  }
}

/** Cancellation must not await a generator return queued behind a permanently blocked next. */
function cancelSnapshot(stream, iterator) {
  try { stream?.destroy?.() } catch { /* Cancellation is best effort after a bounded failure. */ }
  try { Promise.resolve((iterator ?? stream)?.return?.()).catch(() => {}) }
  catch { /* A failing source cannot replace the public generic build error. */ }
}

/** Bound raw binary bytes and every external await before parsing the identified snapshot. */
async function snapshotBytes(readSnapshot, job, budget) {
  const chunks = []
  let size = 0
  let stream
  let iterator
  let completed = false
  const hash = createHash('sha256')
  try {
    budget.remaining()
    const acquisition = Promise.resolve(readSnapshot(job.snapshotKey, { signal: budget.signal }))
    acquisition.then((lateStream) => {
      if (budget.signal.aborted) cancelSnapshot(lateStream)
    }, () => {})
    stream = await budget.wait(acquisition)
    iterator = stream[Symbol.asyncIterator]()
    while (true) {
      budget.remaining()
      const step = await budget.wait(iterator.next())
      if (step.done) { completed = true; break }
      const chunk = step.value
      if (!(chunk instanceof Uint8Array)) throw new Error('Non-binary snapshot')
      size += chunk.byteLength
      if (size > MAX_SNAPSHOT_BYTES) throw new Error('Snapshot exceeds limit')
      const bytes = Buffer.from(chunk)
      chunks.push(bytes)
      hash.update(bytes)
    }
  } finally {
    if (!completed && stream) cancelSnapshot(stream, iterator)
  }
  budget.remaining()
  if (`sha256:${hash.digest('hex')}` !== job.snapshotChecksum) throw new Error('Snapshot checksum mismatch')
  const bytes = Buffer.concat(chunks, size)
  const snapshot = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes))
  if (snapshot?.releaseId !== job.releaseId) throw new Error('Snapshot release mismatch')
  for (const asset of snapshot.assets ?? []) {
    if (typeof asset.src === 'string' && asset.src.startsWith('/') && !asset.src.startsWith('//')) {
      if (fixturePaths.has(decodeURIComponent(new URL(asset.src, 'https://snapshot.invalid').pathname))) {
        throw new Error('Snapshot references development media')
      }
    }
  }
  return bytes
}

/** Check generated references before removing the exact media produced by fixture scripts. */
async function removeFixtureMedia(directory, budget) {
  async function inspect(current) {
    budget.remaining()
    for (const entry of await budget.wait(readdir(current, { withFileTypes: true }))) {
      budget.remaining()
      const target = join(current, entry.name)
      if (entry.isSymbolicLink()) throw new Error('Unsafe static output')
      if (entry.isDirectory()) await inspect(target)
      else if (['.html', '.js', '.mjs', '.json', '.css', '.xml'].includes(extname(entry.name))) {
        const text = (await budget.wait(readFile(target, { encoding: 'utf8', signal: budget.signal }))).replaceAll('\\/', '/')
        if (localFixtureReference.test(text)) throw new Error('Output references development media')
      }
    }
  }
  await inspect(directory)
  for (const name of fixtureNames) {
    budget.remaining()
    await rm(join(directory, 'media', name), { force: true })
    budget.remaining()
  }
}

/** Keep cleanup errors generic without attaching filesystem paths or provider data. */
async function removeSnapshotDirectory(directory) {
  try { await rm(directory, { recursive: true, force: true }) }
  catch { throw new Error('Build failed') }
}

/**
 * Build an immutable reviewed snapshot using trusted source and fixed commands.
 * The returned operation shares a process-wide lock; the worker owns deployment locking.
 * Source, snapshot, and output paths require exclusive local container storage; network shares
 * are unsupported. Mutations finish before the lock is released even after the deadline.
 * @param {{sourceRoot: string, workRoot: string, readSnapshot: Function, run?: Function, timeoutMs?: number}} options
 * @returns {(job: {id: string, releaseId: string, snapshotKey: string, snapshotChecksum: string}) => Promise<string>}
 */
export function createBuilder({ sourceRoot, workRoot, readSnapshot, run = runBuildCommand, timeoutMs = 600_000 }) {
  async function build(job) {
    let snapshotDirectory
    let budget
    try {
      if (!/^build_[a-f0-9]{64}$/.test(job?.id) || !/^rel_[a-z0-9_]+$/.test(job?.releaseId)
        || !/^sha256:[a-f0-9]{64}$/.test(job?.snapshotChecksum)
        || job.snapshotKey !== `snapshots/${job.releaseId}/${job.snapshotChecksum}.json`
        || !Number.isSafeInteger(timeoutMs) || timeoutMs <= 0) throw new Error('Invalid build job')
      budget = buildBudget(timeoutMs)
      const source = await budget.wait(realpath(resolve(sourceRoot)))
      const configuredWork = resolve(workRoot)
      if (inside(source, configuredWork) || configuredWork.split(/[/\\]/).some(part => /^fixtures?$/i.test(part))) {
        throw new Error('Unsafe work directory')
      }
      await mkdir(configuredWork, { recursive: true })
      budget.remaining()
      const work = await budget.wait(realpath(configuredWork))
      if (inside(source, work) || work.split(/[/\\]/).some(part => /^fixtures?$/i.test(part))) throw new Error('Unsafe work directory')
      snapshotDirectory = await mkdtemp(join(work, `${job.id}-`))
      budget.remaining()
      const bytes = await snapshotBytes(readSnapshot, job, budget)
      const snapshotPath = join(snapshotDirectory, 'snapshot.json')
      await writeFile(snapshotPath, bytes, { flag: 'wx', mode: 0o600 })
      budget.remaining()
      const output = await fixedDirectory(source, ['apps', 'web', '.output'], budget)
      const nuxt = await fixedDirectory(source, ['apps', 'web', '.nuxt'], budget)
      await rm(output, { recursive: true, force: true })
      budget.remaining()
      await rm(nuxt, { recursive: true, force: true })
      budget.remaining()
      const env = buildEnvironment(snapshotPath)
      const cli = pnpmCli()
      const execute = async (args) => {
        // Await termination rather than racing the runner: shared output must have one writer.
        await run({ command: process.execPath, args, cwd: source, env: { ...env }, timeoutMs: budget.remaining() })
        budget.remaining()
      }
      await execute(['scripts/require-content-snapshot.mjs'])
      await execute([cli, '--config.verify-deps-before-run=false', '--filter', '@yujian/schema', 'generate'])
      await execute([cli, '--config.verify-deps-before-run=false', '--filter', '@yujian/web', 'generate'])
      const directory = await fixedDirectory(source, ['apps', 'web', '.output', 'public'], budget)
      if (!(await budget.wait(stat(join(directory, 'index.html')))).isFile()) throw new Error('Missing index')
      await removeFixtureMedia(directory, budget)
      await execute(['scripts/verify-static-output.mjs', 'apps/web/.output/public'])
      return directory
    } catch {
      throw new Error('Build failed')
    } finally {
      budget?.dispose()
      if (snapshotDirectory) await removeSnapshotDirectory(snapshotDirectory)
    }
  }
  return (job) => {
    const operation = buildQueue.then(() => build(job))
    buildQueue = operation.catch(() => {})
    return operation
  }
}
