import { existsSync, readFileSync } from 'node:fs'
import { isAbsolute, resolve } from 'node:path'

export interface SnapshotLoadOptions {
  envPath?: string
  workspaceRoot: string
  defaultPath: string
}

/** Resolve the immutable snapshot selected for a static build. */
export function resolveContentSnapshotPath(
  envPath: string | undefined,
  workspaceRoot: string,
  defaultPath: string,
): string {
  const configured = envPath?.trim()
  if (!configured) {
    return defaultPath
  }
  return isAbsolute(configured) ? configured : resolve(workspaceRoot, configured)
}

/** Load a JSON snapshot at build time and fail closed on malformed content. */
export function loadContentSnapshot(path: string): Record<string, unknown> {
  if (!existsSync(path)) {
    throw new Error(`Content snapshot not found: ${path}`)
  }
  let raw: string
  try {
    raw = readFileSync(path, 'utf8')
  } catch (error) {
    throw new Error(`Content snapshot could not be read: ${path}`, { cause: error })
  }
  let value: unknown
  try {
    value = JSON.parse(raw)
  } catch (error) {
    throw new Error(`Content snapshot is invalid JSON: ${path}`, { cause: error })
  }
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw new Error(`Content snapshot must have an object root: ${path}`)
  }
  return value as Record<string, unknown>
}

export function loadBuildSnapshot(options: SnapshotLoadOptions): Record<string, unknown> {
  const path = resolveContentSnapshotPath(options.envPath, options.workspaceRoot, options.defaultPath)
  return loadContentSnapshot(path)
}
