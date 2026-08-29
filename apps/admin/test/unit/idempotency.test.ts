import { describe, expect, it } from 'vitest'
import { createOperationKeyStore } from '../../utils/idempotency'

describe('admin operation idempotency', () => {
  it('reuses a key for the same action and version', () => {
    const values = ['key-1', 'key-2']
    const store = createOperationKeyStore(() => values.shift()!)

    expect(store.get('publish', 'ver_1')).toBe('key-1')
    expect(store.get('publish', 'ver_1')).toBe('key-1')
    expect(store.get('publish', 'ver_2')).toBe('key-2')
  })

  it('isolates publish and rollback keys and supports an explicit reset', () => {
    const values = ['publish-1', 'rollback-1', 'publish-2']
    const store = createOperationKeyStore(() => values.shift()!)

    expect(store.get('publish', 'ver_1')).toBe('publish-1')
    expect(store.get('rollback', 'ver_1')).toBe('rollback-1')
    store.reset('publish', 'ver_1')
    expect(store.get('publish', 'ver_1')).toBe('publish-2')
  })
})
