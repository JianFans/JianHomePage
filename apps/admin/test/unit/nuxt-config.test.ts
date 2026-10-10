import { expect, it } from 'vitest'

it('在不同路径分隔符下内联 Nuxt renderer 运行时', async () => {
  const { loadNuxtConfig } = await import('nuxt/kit')
  const config = await loadNuxtConfig({ cwd: process.cwd() })
  const nuxtRuntimePattern = config.nitro?.externals?.inline?.find(value => value instanceof RegExp)

  expect(nuxtRuntimePattern).toBeInstanceOf(RegExp)
  expect((nuxtRuntimePattern as RegExp).test('C:\\repo\\node_modules\\nuxt\\dist\\runtime\\server.js')).toBe(true)
  expect((nuxtRuntimePattern as RegExp).test('/repo/node_modules/nuxt/dist/runtime/server.js')).toBe(true)
})
