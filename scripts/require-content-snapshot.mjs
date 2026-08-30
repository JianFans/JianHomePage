import { realpathSync, statSync } from 'node:fs'
import { isAbsolute, relative, resolve } from 'node:path'

function requireContentSnapshotPath(value) {
  const configured = value?.trim()
  if (!configured) {
    throw new Error('CONTENT_SNAPSHOT_PATH is required for EdgeOne builds')
  }

  let snapshotPath
  try {
    snapshotPath = realpathSync(resolve(process.cwd(), configured))
  } catch (error) {
    throw new Error('CONTENT_SNAPSHOT_PATH must reference an existing file', { cause: error })
  }
  if (!statSync(snapshotPath).isFile()) {
    throw new Error('CONTENT_SNAPSHOT_PATH must reference a file')
  }

  const fixtureRoot = realpathSync(resolve(process.cwd(), 'content/fixtures'))
  const fixtureRelative = relative(fixtureRoot, snapshotPath)
  if (fixtureRelative === '' || (!fixtureRelative.startsWith('..') && !isAbsolute(fixtureRelative))) {
    throw new Error('CONTENT_SNAPSHOT_PATH must not reference development fixtures')
  }
}

try {
  requireContentSnapshotPath(process.env.CONTENT_SNAPSHOT_PATH)
} catch (error) {
  console.error(error instanceof Error ? error.message : 'CONTENT_SNAPSHOT_PATH is invalid')
  process.exitCode = 1
}
