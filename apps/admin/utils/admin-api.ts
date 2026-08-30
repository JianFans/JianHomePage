import { createIdempotencyKey } from './idempotency'

export interface AdminVersion {
  id: string
  status: 'draft' | 'in_review' | 'publishing' | 'published' | 'archived'
  revision: number
  snapshot: Record<string, unknown>
  checksum: string
  reviewApproved?: boolean
  createdAt?: string
  updatedAt?: string
}

export interface AdminPublishJob {
  id: string
  versionId: string
  status: 'pending' | 'building' | 'succeeded' | 'failed'
  snapshotKey: string
  snapshotChecksum: string
  buildId?: string
  errorMessage?: string
}

export interface AdminApiErrorShape {
  code: string
  message: string
  requestId: string
}

export class AdminApiError extends Error {
  readonly status: number
  readonly code: string
  readonly requestId: string

  constructor(status: number, body: Partial<AdminApiErrorShape>, fallbackMessage = '请求失败') {
    super(body.message || fallbackMessage)
    this.name = 'AdminApiError'
    this.status = status
    this.code = body.code || 'request_failed'
    this.requestId = body.requestId || ''
  }
}

export type FetchLike = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export interface AdminApiOptions {
  baseUrl: string
  token?: string
  fetcher?: FetchLike
}

export function normalizeBaseUrl(value: string): string {
  return value.trim().replace(/\/+$/, '')
}

export function parseSnapshotJSON(value: string): { snapshot: Record<string, unknown> | null; error: string | null } {
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { snapshot: null, error: '快照必须是 JSON 对象' }
    }
    return { snapshot: parsed as Record<string, unknown>, error: null }
  } catch {
    return { snapshot: null, error: 'JSON 格式无效' }
  }
}

export function createAdminApi(options: AdminApiOptions) {
  const baseUrl = normalizeBaseUrl(options.baseUrl)
  const fetcher = options.fetcher || globalThis.fetch

  async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers)
    headers.set('Accept', 'application/json')
    if (init.body && !headers.has('Content-Type')) {
      headers.set('Content-Type', 'application/json')
    }
    if (options.token?.trim()) {
      headers.set('Authorization', `Bearer ${options.token.trim()}`)
    }
    const response = await fetcher(`${baseUrl}${path}`, { ...init, headers })
    if (!response.ok) {
      let body: Partial<AdminApiErrorShape> = {}
      try {
        body = await response.json() as Partial<AdminApiErrorShape>
      } catch {
        // Keep the status-only error safe when the server did not return JSON.
      }
      throw new AdminApiError(response.status, body)
    }
    if (response.status === 204) {
      return undefined as T
    }
    return await response.json() as T
  }

  const json = (value: unknown): RequestInit => ({
    method: 'POST',
    body: JSON.stringify(value),
  })

  return {
    request,
    createVersion(snapshot: Record<string, unknown>) {
      return request<AdminVersion>('/api/v1/versions', json({ snapshot }))
    },
    getVersion(versionId: string) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}`)
    },
    updateVersion(versionId: string, revision: number, snapshot: Record<string, unknown>) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}`, {
        method: 'PUT',
        headers: { 'If-Match': JSON.stringify(String(revision)) },
        body: JSON.stringify({ snapshot }),
      })
    },
    submitReview(versionId: string, revision: number) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/review`, json({ revision }))
    },
    approveReview(versionId: string, revision: number) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/approve`, json({ revision }))
    },
    rejectReview(versionId: string, revision: number, reason: string) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/reject`, json({ revision, reason }))
    },
    publish(versionId: string, idempotencyKey = createIdempotencyKey()) {
      return request<AdminPublishJob>('/api/v1/publishes', {
        ...json({ versionId }),
        headers: { 'Idempotency-Key': idempotencyKey },
      })
    },
    getPublish(publishId: string) {
      return request<AdminPublishJob>(`/api/v1/publishes/${encodeURIComponent(publishId)}`)
    },
    refreshPublish(publishId: string) {
      return request<AdminPublishJob>(`/api/v1/publishes/${encodeURIComponent(publishId)}/refresh`, json({}))
    },
    rollback(versionId: string, idempotencyKey = createIdempotencyKey('rollback')) {
      return request<AdminPublishJob>('/api/v1/rollbacks', {
        ...json({ versionId }),
        headers: { 'Idempotency-Key': idempotencyKey },
      })
    },
  }
}
