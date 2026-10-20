import { afterEach, describe, expect, it, vi } from 'vitest'
import { createEntityId } from '../../utils/identity'

afterEach(() => vi.unstubAllGlobals())

describe('typed UUID identifiers', () => {
  it('preserves the requested prefix and generates distinct UUIDv4 values', () => {
    const first = createEntityId('hero_')
    expect(first).toMatch(/^hero_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/)
    expect(createEntityId('hero_')).not.toBe(first)
  })

  it.each(['', 'hero', 'Hero_', '../hero_'])('rejects invalid prefix %s', (prefix) => {
    expect(() => createEntityId(prefix)).toThrow(TypeError)
  })

  it.each([undefined, {}])('fails explicitly without secure UUID generation', (crypto) => {
    vi.stubGlobal('crypto', crypto)
    expect(() => createEntityId('hero_')).toThrow('Secure UUID generation is unavailable')
  })
})
