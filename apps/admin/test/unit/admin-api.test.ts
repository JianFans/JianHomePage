import { describe, expect, it, vi } from 'vitest'
import { createAdminApi, normalizeBaseUrl, parseSnapshotJSON } from '../../utils/admin-api'

function response(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('admin API client', () => {
  it('normalizes an API base URL without changing the path contract', () => {
    expect(normalizeBaseUrl('  https://api.yujian.me/// ')).toBe('https://api.yujian.me')
  })

  it('sends bearer and optimistic-lock headers for updates', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      expect(init?.method).toBe('PUT')
      const headers = new Headers(init?.headers)
      expect(headers.get('Authorization')).toBe('Bearer session-token')
      expect(headers.get('If-Match')).toBe('"3"')
      return response(200, { id: 'ver_1', revision: 4 })
    })
    const api = createAdminApi({ baseUrl: 'https://api.yujian.me/', token: 'session-token', fetcher })

    const result = await api.updateVersion('ver_1', 3, { schemaVersion: '1.0.0' })

    expect(result.revision).toBe(4)
    expect(fetcher).toHaveBeenCalledWith('https://api.yujian.me/api/v1/versions/ver_1', expect.anything())
  })

  it('sends an explicit idempotency key for publish requests', async () => {
    const fetcher = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
      const headers = new Headers(init?.headers)
      expect(headers.get('Idempotency-Key')).toBe('publish-fixed')
      return response(202, { id: 'pub_1', status: 'building' })
    })
    const api = createAdminApi({ baseUrl: 'https://api.yujian.me', fetcher })

    await api.publish('ver_1', 'publish-fixed')
    expect(fetcher).toHaveBeenCalledTimes(1)
  })

  it('turns structured API errors into safe typed errors', async () => {
    const api = createAdminApi({
      baseUrl: 'https://api.yujian.me',
      fetcher: async () => response(409, { code: 'conflict', message: '冲突', requestId: 'req-1' }),
    })

    await expect(api.getVersion('ver_1')).rejects.toMatchObject({
      status: 409,
      code: 'conflict',
      requestId: 'req-1',
    })
  })

  it('rejects non-object snapshots before a write', () => {
    expect(parseSnapshotJSON('[]')).toEqual({ snapshot: null, error: '快照必须是 JSON 对象' })
    expect(parseSnapshotJSON('{"releaseId":"rel_1"}')).toEqual({ snapshot: { releaseId: 'rel_1' }, error: null })
  })
})
