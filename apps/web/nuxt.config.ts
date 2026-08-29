import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'
import { loadBuildSnapshot } from './utils/build-snapshot'

const appDirectory = fileURLToPath(new URL('.', import.meta.url))
const workspaceRoot = resolve(appDirectory, '../..')
const defaultSnapshotPath = resolve(workspaceRoot, 'content/fixtures/homepage.json')
const buildSnapshot = loadBuildSnapshot({
  envPath: process.env.CONTENT_SNAPSHOT_PATH,
  workspaceRoot,
  defaultPath: defaultSnapshotPath,
})

export default defineNuxtConfig({
  compatibilityDate: '2026-08-01',
  devtools: { enabled: false },
  telemetry: false,
  modules: ['@nuxt/image'],
  css: ['~/assets/css/main.css'],
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
    prerender: {
      routes: ['/sitemap.xml'],
    },
  },
  runtimeConfig: {
    public: {
      contentSnapshot: buildSnapshot,
    },
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
        { property: 'og:title', content: '遇健我 · 王子健' },
        { property: 'og:description', content: '音乐人王子健官方站' },
        { property: 'og:type', content: 'website' },
        { property: 'og:url', content: 'https://yujian.me' },
        { property: 'og:image', content: 'https://yujian.me/media/hero-studio.webp' },
        { property: 'og:image:alt', content: '遇健我 · 王子健' },
        { name: 'twitter:card', content: 'summary_large_image' },
      ],
      link: [
        { rel: 'canonical', href: 'https://yujian.me' },
      ],
    },
  },
})
