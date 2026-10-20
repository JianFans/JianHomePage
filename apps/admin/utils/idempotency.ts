import { createEntityId } from './identity'

export type PublishOperation = 'publish' | 'rollback'
export type PublishTaskStatus = 'pending' | 'building' | 'succeeded' | 'failed'

export function createIdempotencyKey(prefix: PublishOperation = 'publish'): string {
  return createEntityId(`${prefix}_`)
}

export function createOperationKeyStore(
  factory: (operation: PublishOperation) => string = createIdempotencyKey,
) {
  const keys = new Map<string, string>()
  const identity = (operation: PublishOperation, versionId: string) => `${operation}:${versionId}`

  return {
    get(operation: PublishOperation, versionId: string): string {
      const key = identity(operation, versionId)
      const existing = keys.get(key)
      if (existing) return existing
      const created = factory(operation)
      keys.set(key, created)
      return created
    },
    reset(operation: PublishOperation, versionId: string) {
      keys.delete(identity(operation, versionId))
    },
    settle(operation: PublishOperation, versionId: string, status: PublishTaskStatus) {
      if (status === 'succeeded' || status === 'failed') {
        keys.delete(identity(operation, versionId))
      }
    },
  }
}
