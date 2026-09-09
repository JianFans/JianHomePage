import { createIdempotencyKey } from './idempotency'
import type { AdminLocale } from './admin-locale'

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

  /**
   * 将管理 API 的 HTTP 状态与安全错误载荷封装为可识别异常。
   */
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

/**
 * 规范化管理 API 根地址，保留路径并移除首尾空白和尾部斜杠。
 */
export function normalizeBaseUrl(value: string): string {
  return value.trim().replace(/\/+$/, '')
}

export type SnapshotJSONErrorCode = 'invalid-json' | 'object-root'

/**
 * 根据管理界面语言返回稳定的 JSON 编辑器错误消息。
 */
export function snapshotJSONErrorMessage(code: SnapshotJSONErrorCode, locale: AdminLocale = 'zh-CN'): string {
  const messages = locale === 'en'
    ? {
        'invalid-json': 'Invalid JSON',
        'object-root': 'Snapshot must be a JSON object',
      }
    : {
        'invalid-json': 'JSON 格式无效',
        'object-root': '快照必须是 JSON 对象',
      }
  return messages[code]
}

/**
 * 解析快照 JSON，并在写入前确保根节点是普通对象。
 */
export function parseSnapshotJSON(
  value: string,
  locale: AdminLocale = 'zh-CN',
): { snapshot: Record<string, unknown> | null; error: string | null } {
  try {
    const parsed: unknown = JSON.parse(value)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return { snapshot: null, error: snapshotJSONErrorMessage('object-root', locale) }
    }
    return { snapshot: parsed as Record<string, unknown>, error: null }
  } catch {
    return { snapshot: null, error: snapshotJSONErrorMessage('invalid-json', locale) }
  }
}

/**
 * 创建管理 API 客户端，并统一处理鉴权、JSON 和结构化错误响应。
 */
export function createAdminApi(options: AdminApiOptions) {
  const baseUrl = normalizeBaseUrl(options.baseUrl)
  const fetcher = options.fetcher || globalThis.fetch

  /**
   * 执行一个管理 API 请求，并把非成功响应转换为 `AdminApiError`。
   */
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

  /** 为 JSON POST 请求生成一致的请求配置。 */
  const json = (value: unknown): RequestInit => ({
    method: 'POST',
    body: JSON.stringify(value),
  })

  return {
    request,
    /** 创建一个新的内容草稿版本。 */
    createVersion(snapshot: Record<string, unknown>) {
      return request<AdminVersion>('/api/v1/versions', json({ snapshot }))
    },
    /** 按稳定版本 ID 读取内容版本。 */
    getVersion(versionId: string) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}`)
    },
    /** 使用乐观锁修订号更新已有草稿。 */
    updateVersion(versionId: string, revision: number, snapshot: Record<string, unknown>) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}`, {
        method: 'PUT',
        headers: { 'If-Match': JSON.stringify(String(revision)) },
        body: JSON.stringify({ snapshot }),
      })
    },
    /** 将指定草稿提交到审核状态。 */
    submitReview(versionId: string, revision: number) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/review`, json({ revision }))
    },
    /** 批准处于审核中的内容版本。 */
    approveReview(versionId: string, revision: number) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/approve`, json({ revision }))
    },
    /** 将审核中的版本退回并记录原因。 */
    rejectReview(versionId: string, revision: number, reason: string) {
      return request<AdminVersion>(`/api/v1/versions/${encodeURIComponent(versionId)}/reject`, json({ revision, reason }))
    },
    /** 使用幂等键创建发布任务。 */
    publish(versionId: string, idempotencyKey = createIdempotencyKey()) {
      return request<AdminPublishJob>('/api/v1/publishes', {
        ...json({ versionId }),
        headers: { 'Idempotency-Key': idempotencyKey },
      })
    },
    /** 读取发布任务的当前状态。 */
    getPublish(publishId: string) {
      return request<AdminPublishJob>(`/api/v1/publishes/${encodeURIComponent(publishId)}`)
    },
    /** 请求服务端刷新外部构建状态。 */
    refreshPublish(publishId: string) {
      return request<AdminPublishJob>(`/api/v1/publishes/${encodeURIComponent(publishId)}/refresh`, json({}))
    },
    /** 使用独立幂等键创建版本回滚任务。 */
    rollback(versionId: string, idempotencyKey = createIdempotencyKey('rollback')) {
      return request<AdminPublishJob>('/api/v1/rollbacks', {
        ...json({ versionId }),
        headers: { 'Idempotency-Key': idempotencyKey },
      })
    },
  }
}
