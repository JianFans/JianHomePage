import { createHash } from 'node:crypto'
import { existsSync, readFileSync, statSync } from 'node:fs'
import { extname, isAbsolute, relative, resolve } from 'node:path'
import { assertContentSnapshot, type YujianContentSnapshot } from '@yujian/schema'

export interface SnapshotLoadOptions {
  envPath?: string
  workspaceRoot: string
  defaultPath: string
  publicDirectory?: string
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
export function loadContentSnapshot(path: string): YujianContentSnapshot {
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
  try {
    assertContentSnapshot(value)
  } catch (error) {
    const detail = error instanceof Error ? error.message : 'unknown validation error'
    throw new Error(`${detail}: ${path}`, { cause: error })
  }
  return value
}

export function loadBuildSnapshot(options: SnapshotLoadOptions): YujianContentSnapshot {
  const path = resolveContentSnapshotPath(options.envPath, options.workspaceRoot, options.defaultPath)
  const snapshot = loadContentSnapshot(path)
  if (options.publicDirectory) {
    assertLocalAssetFiles(snapshot, options.publicDirectory)
  }
  return snapshot
}

function assertLocalAssetFiles(snapshot: YujianContentSnapshot, publicDirectory: string): void {
  const root = resolve(publicDirectory)
  const mimeByExtension: Record<string, string> = {
    '.gif': 'image/gif',
    '.mp3': 'audio/mpeg',
    '.mp4': 'video/mp4',
    '.wav': 'audio/wav',
    '.webp': 'image/webp',
  }
  for (const asset of snapshot.assets) {
    if (!asset.src.startsWith('/') || asset.src.startsWith('//')) continue
    const reference = decodeURIComponent(asset.src.split(/[?#]/, 1)[0]!).replace(/^\/+/, '')
    const target = resolve(root, reference)
    const relativePath = relative(root, target)
    if (relativePath.startsWith('..') || isAbsolute(relativePath)) {
      throw new Error(`Local asset path escapes public directory: ${asset.src}`)
    }
    if (!existsSync(target)) {
      throw new Error(`Local asset file not found: ${asset.src}`)
    }
    const stats = statSync(target)
    if (!stats.isFile()) {
      throw new Error(`Local asset file not found: ${asset.src}`)
    }
    if (stats.size !== asset.byteSize) {
      throw new Error(`Local asset byte size mismatch: ${asset.src}`)
    }
    const checksum = `sha256:${createHash('sha256').update(readFileSync(target)).digest('hex')}`
    if (checksum !== asset.checksum) {
      throw new Error(`Local asset checksum mismatch: ${asset.src}`)
    }
    const expectedMime = mimeByExtension[extname(target).toLowerCase()]
    if (!expectedMime || expectedMime !== asset.mimeType) {
      throw new Error(`Local asset MIME mismatch: ${asset.src}`)
    }
  }
}
