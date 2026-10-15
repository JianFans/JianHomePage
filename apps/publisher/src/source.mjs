import { execFile } from 'node:child_process'
import { readFile } from 'node:fs/promises'
import { join } from 'node:path'
import { promisify } from 'node:util'

const execute = promisify(execFile)

/** Verify the image stamp, or a clean local checkout, before accepting frozen code identities. */
export async function verifySourceRevision(sourceRoot, revision) {
  let actual
  try {
    actual = (await readFile(join(sourceRoot, '.publisher-revision'), 'utf8')).trim()
  } catch (error) {
    if (error.code !== 'ENOENT') throw new Error('Source revision verification failed', { cause: error })
    try {
      const options = { cwd: sourceRoot, timeout: 10000, maxBuffer: 65536, windowsHide: true }
      actual = (await execute('git', ['rev-parse', 'HEAD'], options)).stdout.trim()
      const dirty = (await execute('git', ['status', '--porcelain', '--untracked-files=normal'], options)).stdout.trim()
      if (dirty) throw new Error('Dirty source', { cause: error })
    } catch { throw new Error('Source revision requires an immutable image or clean checkout') }
  }
  if (!/^[a-f0-9]{40}$/.test(actual) || actual !== revision) throw new Error('Source revision mismatch')
}
