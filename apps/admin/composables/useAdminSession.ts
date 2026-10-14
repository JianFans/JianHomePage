import { useRuntimeConfig } from '#imports'
import { computed, onScopeDispose, ref, shallowRef } from 'vue'
import { createAdminApi, normalizeBaseUrl, type AdminSession, type FetchLike } from '../utils/admin-api'

/** 恢复与撤销服务端 HttpOnly 会话；凭据只存在当前组件作用域。 */
export function useAdminSession(options: { baseUrl?: string; fetcher?: FetchLike } = {}) {
  const runtime = useRuntimeConfig()
  const baseUrl = normalizeBaseUrl(options.baseUrl ?? String(runtime.public.apiBaseUrl || ''))
  const state = shallowRef<AdminSession | null>(null)
  const busy = ref(false)
  const error = ref('')
  const expired = ref(false)
  const authenticated = computed(() => state.value?.authenticated === true)
  const principal = computed(() => authenticated.value ? state.value?.principal ?? null : null)
  const csrfToken = computed(() => authenticated.value ? state.value?.csrfToken ?? '' : '')
  const loginAvailable = computed(() => state.value?.loginAvailable === true)
  const loginUrl = `${baseUrl}/api/v1/auth/login`
  let sequence = 0
  let disposed = false
  let timer: ReturnType<typeof setTimeout> | undefined

  /** 清理定时撤下身份的任务，防止已退出会话影响后续恢复。 */
  function clearExpiry() {
    if (timer) clearTimeout(timer)
    timer = undefined
  }

  /** 使所有迟到请求失效并撤下身份，不清理调用方尚未保存的内容。 */
  function expire() {
    sequence++
    clearExpiry()
    state.value = state.value ? { authenticated: false, loginAvailable: state.value.loginAvailable } : null
    busy.value = false
    expired.value = true
  }

  /** 请求明确的会话状态，并只接受最新请求的有效结果。 */
  async function restore() {
    if (disposed) return
    const current = ++sequence
    busy.value = true
    error.value = ''
    try {
      const next = await createAdminApi({ baseUrl, fetcher: options.fetcher }).getSession()
      if (disposed || current !== sequence) return
      const expires = Date.parse(next.expiresAt || '')
      if (typeof next.authenticated !== 'boolean' || typeof next.loginAvailable !== 'boolean'
        || (next.authenticated && (!next.principal?.subject || !Array.isArray(next.principal.roles)
          || !next.csrfToken || !Number.isFinite(expires) || expires <= Date.now()))) {
        throw new Error('Invalid session response')
      }
      clearExpiry()
      state.value = next
      if (next.authenticated) expired.value = false
      if (next.authenticated) timer = setTimeout(expire, Math.min(expires - Date.now(), 2147483647))
    } catch {
      if (disposed || current !== sequence) return
      error.value = 'session-unavailable'
    } finally {
      if (!disposed && current === sequence) busy.value = false
    }
  }

  /** 仅在服务端撤销成功后移除身份，网络失败保持可重试状态。 */
  async function logout(): Promise<boolean> {
    if (busy.value || disposed) return false
    const current = ++sequence
    busy.value = true
    error.value = ''
    try {
      await createAdminApi({ baseUrl, csrfToken: csrfToken.value, fetcher: options.fetcher }).logout()
      if (disposed || current !== sequence) return false
      clearExpiry()
      state.value = { authenticated: false, loginAvailable: loginAvailable.value }
      expired.value = false
      return true
    } catch {
      if (!disposed && current === sequence) error.value = 'logout-failed'
      return false
    } finally {
      if (!disposed && current === sequence) busy.value = false
    }
  }

  onScopeDispose(() => { disposed = true; sequence++; clearExpiry() })
  return { authenticated, principal, csrfToken, loginAvailable, loginUrl, busy, error, expired, restore, logout, expire }
}
