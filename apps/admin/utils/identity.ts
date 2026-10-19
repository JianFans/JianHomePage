/** 生成带类型前缀的标准 UUIDv4；不支持安全随机源时明确失败。 */
export function createEntityId(prefix: string): string {
  if (!/^[a-z][a-z0-9]*_$/.test(prefix)) throw new TypeError('Invalid identifier prefix')
  if (!globalThis.crypto || typeof globalThis.crypto.randomUUID !== 'function') {
    throw new Error('Secure UUID generation is unavailable')
  }
  return `${prefix}${globalThis.crypto.randomUUID()}`
}
