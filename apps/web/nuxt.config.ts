export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  devtools: { enabled: false },
  modules: ['@nuxt/image'],
  build: {
    transpile: ['vue-i18n'],
  },
  vite: {
    define: {
      __VUE_I18N_FULL_INSTALL__: true,
      __VUE_I18N_LEGACY_API__: false,
      __INTLIFY_DROP_MESSAGE_COMPILER__: false,
      __INTLIFY_PROD_DEVTOOLS__: false,
      __VUE_PROD_DEVTOOLS__: false,
    },
  },
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
