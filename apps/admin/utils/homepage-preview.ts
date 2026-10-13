import { diagnoseContentSnapshot, type YujianContentSnapshot } from '@yujian/schema'
import type { AdminLocale } from './admin-locale'

const CHANNEL = 'yujian-homepage-preview-v1'

export type PreviewMessage =
  | { channel: typeof CHANNEL; type: 'ready' }
  | { channel: typeof CHANNEL; type: 'snapshot'; snapshot: YujianContentSnapshot; locale: AdminLocale }
  | { channel: typeof CHANNEL; type: 'locale'; locale: AdminLocale }

/** Construct the handshake explicitly, excluding all management workspace state. */
export function createReadyMessage(): Extract<PreviewMessage, { type: 'ready' }> {
  return { channel: CHANNEL, type: 'ready' }
}

/** Construct a detached, JSON-only transfer object containing only content and locale. */
export function createSnapshotMessage(input: { snapshot: YujianContentSnapshot; locale: AdminLocale }): Extract<PreviewMessage, { type: 'snapshot' }> {
  return { channel: CHANNEL, type: 'snapshot', snapshot: cloneSnapshot(input.snapshot), locale: input.locale }
}

/** Exchange a preview-only language preference without persistence or URL changes. */
export function createLocaleMessage(locale: AdminLocale): Extract<PreviewMessage, { type: 'locale' }> {
  return { channel: CHANNEL, type: 'locale', locale }
}

/** Validate both browser identities and the full canonical contract before accepting content. */
export function readPreviewMessage(event: MessageEvent, origin: string, source: Window): PreviewMessage | null {
  if (event.origin !== origin || event.source !== source) return null
  const data: unknown = event.data
  if (!data || typeof data !== 'object' || Array.isArray(data)) return null
  const value = data as Record<string, unknown>
  if (value.channel !== CHANNEL) return null
  if (value.type === 'ready' && hasOnlyKeys(value, ['channel', 'type'])) return createReadyMessage()
  if (value.locale !== 'zh-CN' && value.locale !== 'en') return null
  if (value.type === 'locale' && hasOnlyKeys(value, ['channel', 'type', 'locale'])) return createLocaleMessage(value.locale)
  if (value.type !== 'snapshot' || !hasOnlyKeys(value, ['channel', 'type', 'snapshot', 'locale'])) return null
  try {
    if (diagnoseContentSnapshot(value.snapshot).length) return null
    const snapshot = cloneSnapshot(value.snapshot as YujianContentSnapshot)
    for (const asset of snapshot.assets) {
      if (asset.src.startsWith('/')) asset.src = new URL(asset.src, snapshot.site.canonicalUrl).href
    }
    return { channel: CHANNEL, type: 'snapshot', snapshot, locale: value.locale }
  } catch {
    return null
  }
}

/** Reject unknown transfer fields instead of silently accepting credential-bearing envelopes. */
function hasOnlyKeys(value: Record<string, unknown>, keys: string[]): boolean {
  return Object.keys(value).length === keys.length && keys.every(key => Object.hasOwn(value, key))
}

/** JSON cloning also unwraps Vue proxies without retaining references to editor content. */
function cloneSnapshot(snapshot: YujianContentSnapshot): YujianContentSnapshot {
  return JSON.parse(JSON.stringify(snapshot)) as YujianContentSnapshot
}
