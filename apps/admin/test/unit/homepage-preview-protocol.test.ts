import type { YujianContentSnapshot } from '@yujian/schema'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import { createLocaleMessage, createReadyMessage, createSnapshotMessage, readPreviewMessage } from '../../utils/homepage-preview'

const origin = 'https://admin.example.com'
const source = {} as Window
const snapshot = fixture as unknown as YujianContentSnapshot

/** Deliver a protocol object through the same event boundary as the browser. */
function receive(data: unknown, overrides: Partial<MessageEvent> = {}) {
  return readPreviewMessage({ data, origin, source, ...overrides } as MessageEvent, origin, source)
}

describe('首页预览协议', () => {
  it('只构造允许的传输字段，隔离传入的连接与审核元数据', () => {
    const input = { snapshot, locale: 'en' as const, token: 'private', apiBaseUrl: 'https://api.example.com', version: { status: 'in_review' } }
    const message = createSnapshotMessage(input)
    expect(Object.keys(message).sort()).toEqual(['channel', 'locale', 'snapshot', 'type'])
    expect(message.snapshot).toEqual(snapshot)
    expect(message.snapshot).not.toBe(snapshot)
    expect(createReadyMessage()).toEqual({ channel: 'yujian-homepage-preview-v1', type: 'ready' })
    expect(createLocaleMessage('en')).toEqual({ channel: 'yujian-homepage-preview-v1', type: 'locale', locale: 'en' })
  })

  it('拒绝错误 origin、source、channel、type、locale 和非 canonical 快照', () => {
    const message = createSnapshotMessage({ snapshot, locale: 'zh-CN' })
    expect(receive(message)).not.toBeNull()
    expect(receive(message, { origin: 'https://other.example.com' })).toBeNull()
    expect(receive(message, { source: {} as Window })).toBeNull()
    expect(receive({ ...message, channel: 'other' })).toBeNull()
    expect(receive({ ...message, type: 'save' })).toBeNull()
    expect(receive({ ...message, locale: 'fr' })).toBeNull()
    expect(receive({ ...message, snapshot: {} })).toBeNull()
    expect(receive({ ...message, snapshot: { ...snapshot, token: 'private' } })).toBeNull()
    expect(receive({ ...message, token: 'private' })).toBeNull()
    expect(receive(null)).toBeNull()
  })

  it('接收时复制快照并将根路径素材解析到公开站域名，不改动原数据', () => {
    const message = createSnapshotMessage({ snapshot, locale: 'en' })
    const root = message.snapshot.assets.find(asset => asset.src.startsWith('/'))!
    const before = root.src
    const received = receive(message)
    expect(received?.type).toBe('snapshot')
    if (received?.type !== 'snapshot') throw new Error('Missing snapshot')
    expect(received.snapshot.assets.find(asset => asset.id === root.id)?.src).toBe(new URL(before, snapshot.site.canonicalUrl).href)
    expect(root.src).toBe(before)
    expect(received.snapshot).not.toBe(message.snapshot)
  })
})
