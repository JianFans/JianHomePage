import { describe, expect, it } from 'vitest'
import { persistAdminLocale, resolveAdminLocale } from '../../utils/admin-locale'

describe('admin locale storage', () => {
  it('falls back to browser language when storage access fails', () => {
    const storage = { getItem: () => { throw new Error('blocked') } }
    expect(resolveAdminLocale(storage, 'en-US')).toBe('en')
  })

  it('keeps locale switching usable when persistence fails', () => {
    const storage = { setItem: () => { throw new Error('quota exceeded') } }
    expect(() => persistAdminLocale('zh-CN', storage)).not.toThrow()
  })
})
