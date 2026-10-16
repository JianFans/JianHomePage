import { afterEach, expect, it, vi } from 'vitest'

afterEach(() => vi.unstubAllEnvs())

it('生产构建默认同源 API，并静态生成独立预览页面', async () => {
  vi.stubEnv('NODE_ENV', 'production')
  vi.stubEnv('ADMIN_API_BASE_URL', '')
  const { loadNuxtConfig } = await import('nuxt/kit')
  const config = await loadNuxtConfig({ cwd: process.cwd() })
  expect(config.runtimeConfig?.public?.apiBaseUrl).toBe('/')
  expect(config.nitro?.prerender?.routes).toContain('/preview')
})

it('独立开发服务和显式生产 API 地址仍可配置', async () => {
  const { loadNuxtConfig } = await import('nuxt/kit')
  vi.stubEnv('NODE_ENV', 'development')
  vi.stubEnv('ADMIN_API_BASE_URL', '')
  const development = await loadNuxtConfig({ cwd: process.cwd() })
  expect(development.runtimeConfig?.public?.apiBaseUrl).toBe('http://127.0.0.1:8080')
  vi.stubEnv('NODE_ENV', 'production')
  vi.stubEnv('ADMIN_API_BASE_URL', 'https://admin.example.test')
  const production = await loadNuxtConfig({ cwd: process.cwd() })
  expect(production.runtimeConfig?.public?.apiBaseUrl).toBe('https://admin.example.test')
})

it('在不同路径分隔符下内联 Nuxt renderer 运行时', async () => {
  const { loadNuxtConfig } = await import('nuxt/kit')
  const config = await loadNuxtConfig({ cwd: process.cwd() })
  const nuxtRuntimePattern = config.nitro?.externals?.inline?.find(value => value instanceof RegExp)
  expect(config.css).not.toContain('~/assets/css/main.css')

  expect(nuxtRuntimePattern).toBeInstanceOf(RegExp)
  expect((nuxtRuntimePattern as RegExp).test('C:\\repo\\node_modules\\nuxt\\dist\\runtime\\server.js')).toBe(true)
  expect((nuxtRuntimePattern as RegExp).test('/repo/node_modules/nuxt/dist/runtime/server.js')).toBe(true)
})
