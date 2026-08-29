export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  devtools: { enabled: false },
  modules: ['@nuxt/image'],
  nitro: {
    preset: 'static',
  },
  ssr: true,
  typescript: {
    strict: true,
    typeCheck: true,
  },
  app: {
    head: {
      htmlAttrs: { lang: 'zh-CN' },
      title: '遇健我 | 王子健官方网站',
      meta: [
        {
          name: 'description',
          content: '遇健我，音乐人王子健官方网站。',
        },
      ],
    },
  },
})

