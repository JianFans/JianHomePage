import { fileURLToPath } from 'node:url'
import { resolve } from 'node:path'
import { loadBuildSnapshot } from './utils/build-snapshot'

const appDirectory = fileURLToPath(new URL('.', import.meta.url))
const workspaceRoot = resolve(appDirectory, '../..')
const defaultSnapshotPath = resolve(workspaceRoot, 'content/fixtures/homepage.json')
const publicDirectory = resolve(appDirectory, 'public')
const buildSnapshot = loadBuildSnapshot({
  envPath: process.env.CONTENT_SNAPSHOT_PATH,
  workspaceRoot,
  defaultPath: defaultSnapshotPath,
  publicDirectory,
})
const defaultLocale = buildSnapshot.site.defaultLocale
const seoTitle = buildSnapshot.site.seo.title[defaultLocale]
const seoDescription = buildSnapshot.site.seo.description[defaultLocale]
const seoAsset = buildSnapshot.assets.find(asset => asset.id === buildSnapshot.site.seo.ogAssetId)
if (!seoAsset) {
  throw new Error(`SEO asset not found: ${buildSnapshot.site.seo.ogAssetId}`)
}
const seoImage = new URL(seoAsset.src, buildSnapshot.site.canonicalUrl).toString()
const seoImageAlt = seoAsset.alt[defaultLocale]

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
      routes: ['/robots.txt', '/sitemap.xml'],
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
      title: seoTitle,
      meta: [
        {
          name: 'description',
          content: seoDescription,
        },
        { property: 'og:title', content: seoTitle },
        { property: 'og:description', content: seoDescription },
        { property: 'og:type', content: 'website' },
        { property: 'og:url', content: buildSnapshot.site.canonicalUrl },
        { property: 'og:image', content: seoImage },
        { property: 'og:image:alt', content: seoImageAlt },
        { name: 'twitter:card', content: 'summary_large_image' },
      ],
      link: [
        { rel: 'canonical', href: buildSnapshot.site.canonicalUrl },
      ],
    },
  },
})
