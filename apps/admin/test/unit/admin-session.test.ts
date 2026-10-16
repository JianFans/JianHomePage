import { mountSuspended } from '@nuxt/test-utils/runtime'
import { defineComponent, reactive } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useAdminSession } from '../../composables/useAdminSession'

/** 在组件作用域管理会话的计时器与迟到请求。 */
async function host(fetcher: typeof fetch, baseUrl = 'https://api.yujian.me') {
  const wrapper = await mountSuspended(defineComponent({
    setup() { return { session: reactive(useAdminSession({ baseUrl, fetcher })) } },
    template: '<div />',
  }))
  return { wrapper, session: wrapper.vm.session }
}

/** 创建可重复读取的会话响应，每个调用独立持有响应体。 */
function sessionResponse(authenticated = true) {
  return new Response(JSON.stringify({ authenticated, loginAvailable: true, ...(authenticated ? {
    principal: { subject: 'editor', name: '编辑', roles: ['editor'] }, csrfToken: 'csrf-private',
    expiresAt: new Date(Date.now() + 60000).toISOString(),
  } : {}) }), { status: 200 })
}

afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); localStorage.clear() })

describe('管理端登录会话', () => {
  it('内嵌管理页面使用同源登录与会话接口', async () => {
    const fetcher = vi.fn(async () => sessionResponse())
    const { wrapper, session } = await host(fetcher, '/')
    await session.restore()
    expect(session.authenticated).toBe(true)
    expect(session.loginUrl).toBe('/api/v1/auth/login')
    expect(fetcher).toHaveBeenCalledWith('/api/v1/auth/session', expect.objectContaining({ credentials: 'include' }))
    wrapper.unmount()
  })

  it('恢复真实会话，只在内存保留身份与 CSRF', async () => {
    const fetcher = vi.fn(async () => sessionResponse())
    const { wrapper, session } = await host(fetcher)
    await session.restore()
    expect(session.authenticated).toBe(true)
    expect(session.principal?.subject).toBe('editor')
    expect(session.csrfToken).toBe('csrf-private')
    expect(localStorage.length).toBe(0)
    expect(session.loginUrl).toBe('https://api.yujian.me/api/v1/auth/login')
    wrapper.unmount()
  })

  it('退出失败保留身份，成功退出才清除会话', async () => {
    const fetcher = vi.fn().mockImplementationOnce(async () => sessionResponse())
      .mockResolvedValueOnce(new Response('{}', { status: 500 }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { wrapper, session } = await host(fetcher)
    await session.restore()
    expect(await session.logout()).toBe(false)
    expect(session.authenticated).toBe(true)
    expect(await session.logout()).toBe(true)
    expect(session.authenticated).toBe(false)
    expect(session.csrfToken).toBe('')
    expect(new Headers(fetcher.mock.calls[1]![1].headers).get('X-CSRF-Token')).toBe('csrf-private')
    wrapper.unmount()
  })

  it('身份过期或销毁后，旧恢复请求不能重新授予界面访问', async () => {
    let finish!: (value: Response) => void
    const fetcher = vi.fn(() => new Promise<Response>(resolve => { finish = resolve }))
    const { wrapper, session } = await host(fetcher)
    const restoring = session.restore()
    session.expire()
    finish(sessionResponse())
    await restoring
    expect(session.authenticated).toBe(false)
    expect(session.csrfToken).toBe('')
    const later = session.restore()
    wrapper.unmount()
    finish(sessionResponse())
    await later
    expect(session.authenticated).toBe(false)
  })

  it('在服务端声明的到期时刻撤下会话，恢复失败可重试', async () => {
    const fetcher = vi.fn().mockRejectedValueOnce(new TypeError('offline')).mockImplementation(async () => sessionResponse())
    const { wrapper, session } = await host(fetcher)
    vi.useFakeTimers()
    await session.restore()
    expect(session.error).toBeTruthy()
    expect(session.authenticated).toBe(false)
    await session.restore()
    expect(session.error).toBe('')
    expect(session.authenticated).toBe(true)
    await vi.advanceTimersByTimeAsync(60001)
    expect(session.authenticated).toBe(false)
    wrapper.unmount()
  })
})
