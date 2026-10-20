import { describe, expect, it } from 'vitest'
import { createIdempotencyKey, createOperationKeyStore } from '../../utils/idempotency'

describe('admin operation idempotency', () => {
  it('为发布和回滚生成带类型前缀的 UUIDv4', () => {
    expect(createIdempotencyKey()).toMatch(/^publish_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    expect(createIdempotencyKey('rollback')).toMatch(/^rollback_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
  })

  it('reuses a key for the same action and version', () => {
    const values = ['key-1', 'key-2']
    const store = createOperationKeyStore(() => values.shift()!)

    expect(store.get('publish', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('key-1')
    expect(store.get('publish', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('key-1')
    expect(store.get('publish', 'ver_4804f8e3-7616-4062-ba6b-9c818b68c23d')).toBe('key-2')
  })

  it('isolates publish and rollback keys and supports an explicit reset', () => {
    const values = ['publish-1', 'rollback-1', 'publish-2']
    const store = createOperationKeyStore(() => values.shift()!)

    expect(store.get('publish', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('publish-1')
    expect(store.get('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('rollback-1')
    store.reset('publish', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')
    expect(store.get('publish', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('publish-2')
  })

  it('rotates a key only after the publish task reaches a terminal state', () => {
    const values = ['rollback-1', 'rollback-2']
    const store = createOperationKeyStore(() => values.shift()!)

    expect(store.get('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('rollback-1')
    store.settle('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d', 'building')
    expect(store.get('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('rollback-1')
    store.settle('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d', 'succeeded')
    expect(store.get('rollback', 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toBe('rollback-2')
  })
})
