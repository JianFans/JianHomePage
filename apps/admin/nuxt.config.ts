export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  devtools: { enabled: false },
  telemetry: false,
  ssr: false,
  nitro: {
    prerender: { routes: ['/preview'] },
    externals: {
      inline: [/[/\\]nuxt[/\\]dist[/\\]/],
    },
  },
  typescript: {
    strict: true,
    typeCheck: true,
  },
  runtimeConfig: {
    public: {
      apiBaseUrl: process.env.ADMIN_API_BASE_URL || (process.env.NODE_ENV === 'production' ? '/' : 'http://127.0.0.1:8080'),
    },
  },
  app: {
    head: {
      htmlAttrs: { lang: 'zh-CN' },
      title: '遇健我 · 内容管理',
      link: [{ rel: 'icon', type: 'image/svg+xml', href: '/favicon.svg' }],
      meta: [
        { name: 'description', content: '遇健我官方站内容与发布管理端。' },
        { name: 'robots', content: 'noindex,nofollow' },
      ],
    },
  },
})
