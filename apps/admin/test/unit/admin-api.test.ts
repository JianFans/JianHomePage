import { describe, expect, it, vi } from 'vitest'
import { createAdminApi, normalizeBaseUrl, parseSnapshotJSON } from '../../utils/admin-api'

/** 构造管理 API 客户端测试使用的 JSON 响应。 */
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

  it('encodes asset list filters and keeps bearer authentication on the management API', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe('https://api.yujian.me/api/v1/assets?status=ready&limit=25&cursor=a%2Fb%3F')
      expect(new Headers(init?.headers).get('Authorization')).toBe('Bearer session-token')
      return response(200, { items: [], nextCursor: 'next' })
    })
    const api = createAdminApi({ baseUrl: 'https://api.yujian.me', token: 'session-token', fetcher })

    await expect(api.listAssets({ status: 'ready', limit: 25, cursor: 'a/b?' })).resolves.toEqual({
      items: [],
      nextCursor: 'next',
    })
  })

  it('creates and completes uploads through authenticated management requests', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const headers = new Headers(init?.headers)
      expect(headers.get('Authorization')).toBe('Bearer session-token')
      if (url.endsWith('/api/v1/assets/uploads')) {
        expect(init?.method).toBe('POST')
        expect(JSON.parse(String(init?.body))).toMatchObject({ fileName: 'cover.webp', checksum: 'sha256:test' })
        return response(201, { asset: { id: 'asset/a' }, uploadUrl: 'https://upload.example.test/signed', expiresAt: '2026-10-09T09:00:00Z' })
      }
      expect(url).toBe('https://api.yujian.me/api/v1/assets/asset%2Fa/complete')
      return response(200, { id: 'asset/a', status: 'ready' })
    })
    const api = createAdminApi({ baseUrl: 'https://api.yujian.me', token: 'session-token', fetcher })

    const upload = await api.createAssetUpload({
      fileName: 'cover.webp', contentType: 'image/webp', size: 1, checksum: 'sha256:test',
      rights: { source: { 'zh-CN': '官方授权' } },
    })
    await expect(api.completeAssetUpload(upload.asset.id)).resolves.toMatchObject({ status: 'ready' })
  })

  it('uploads the blob with only signed headers and never leaks the bearer token', async () => {
    const file = new Blob(['asset'], { type: 'image/webp' })
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(String(input)).toBe('https://upload.example.test/signed')
      expect(init?.method).toBe('PUT')
      expect(init?.body).toBe(file)
      const headers = new Headers(init?.headers)
      expect(headers.get('X-Yujian-Checksum')).toBe('sha256:test')
      expect(headers.get('Content-Type')).toBe('image/webp')
      expect(headers.has('Authorization')).toBe(false)
      expect(headers.has('Accept')).toBe(false)
      return new Response(null, { status: 200 })
    })
    const api = createAdminApi({ baseUrl: 'https://api.yujian.me', token: 'session-token', fetcher })

    await api.uploadAssetBlob({
      asset: { id: 'asset_1', src: '', status: 'pending', metadata: {}, rights: { source: { 'zh-CN': '官方授权' } } },
      uploadUrl: 'https://upload.example.test/signed',
      headers: { 'X-Yujian-Checksum': 'sha256:test', 'Content-Type': 'image/webp' },
      expiresAt: '2026-10-09T09:00:00Z',
    }, file)
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
    expect(parseSnapshotJSON('[]', 'en')).toEqual({ snapshot: null, error: 'Snapshot must be a JSON object' })
    expect(parseSnapshotJSON('{', 'en')).toEqual({ snapshot: null, error: 'Invalid JSON' })
    expect(parseSnapshotJSON('{"releaseId":"rel_1"}')).toEqual({ snapshot: { releaseId: 'rel_1' }, error: null })
  })
})
