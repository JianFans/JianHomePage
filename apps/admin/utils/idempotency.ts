export type PublishOperation = 'publish' | 'rollback'

export function createIdempotencyKey(prefix: PublishOperation = 'publish'): string {
  const random = typeof crypto !== 'undefined' && 'randomUUID' in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `${prefix}-${random}`
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
  }
}
