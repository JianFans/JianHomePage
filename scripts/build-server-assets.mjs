import { createRequire } from 'node:module'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { dirname, resolve } from 'node:path'
import { cp, mkdir, readdir, readFile, rm, stat, writeFile } from 'node:fs/promises'
import { createHash } from 'node:crypto'
import { spawnSync } from 'node:child_process'

const root = fileURLToPath(new URL('..', import.meta.url))
const web = resolve(root, 'apps/web')
const require = createRequire(resolve(web, 'package.json'))
const { build } = await import(pathToFileURL(require.resolve('vite')).href)
const { default: vue } = await import(pathToFileURL(require.resolve('@vitejs/plugin-vue')).href)
const bundle = resolve(root, 'apps/server/internal/webassets/bundle')
await mkdir(bundle, { recursive: true })
const definitions = {
  __VUE_OPTIONS_API__: true,
  __VUE_PROD_DEVTOOLS__: false,
  __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: false,
  'process.env.NODE_ENV': JSON.stringify('production'),
}

await build({
  configFile: false, root: web, base: '/site/', plugins: [vue()],
  define: { ...definitions, 'import.meta.client': true },
  build: { outDir: resolve(bundle, 'public/site'), emptyOutDir: true, copyPublicDir: false, manifest: true, cssCodeSplit: false,
    rollupOptions: { input: resolve(web, 'standalone/client.ts') } },
})

if (!process.argv.includes('--public-only')) {
  const admin = resolve(root, 'apps/admin')
  const adminRequire = createRequire(resolve(admin, 'package.json'))
  const packagePath = adminRequire.resolve('nuxt/package.json')
  const { bin } = JSON.parse(await readFile(packagePath, 'utf8'))
  const result = spawnSync(process.execPath, [resolve(dirname(packagePath), typeof bin === 'string' ? bin : bin.nuxt), 'generate'], {
    cwd: admin, stdio: 'inherit', env: { ...process.env, NODE_ENV: 'production', ADMIN_API_BASE_URL: '/' },
  })
  if (result.error) throw result.error
  if (result.status !== 0) throw new Error('Management application generation failed')
  const adminOutput = resolve(admin, '.output/public')
  for (const route of ['index.html', 'preview/index.html']) {
    const html = await readFile(resolve(adminOutput, route), 'utf8')
    for (const [, asset] of html.matchAll(/(?:src|href)="(\/_nuxt\/[^"?#]+)"/g)) {
      if (asset.includes('..') || !(await stat(resolve(adminOutput, `.${asset}`))).isFile()) throw new Error('Incomplete management browser assets')
    }
  }
  await rm(resolve(bundle, 'admin'), { recursive: true, force: true })
  await cp(adminOutput, resolve(bundle, 'admin'), { recursive: true })
  const adminBundle = resolve(bundle, 'admin')
  const files = []
  /** Record every route dependency, including chunks absent from HTML preload links. */
  async function inventory(directory = '') {
    const entries = await readdir(resolve(adminBundle, directory), { withFileTypes: true })
    entries.sort((left, right) => left.name.localeCompare(right.name, 'en'))
    for (const entry of entries) {
      const path = directory ? `${directory}/${entry.name}` : entry.name
      if (entry.name.startsWith('.') || /[\\:?#%]|\p{Cc}/u.test(entry.name) || path === 'asset-manifest.json') throw new Error('Unsafe management asset path')
      if (entry.isDirectory()) { await inventory(path); continue }
      if (!entry.isFile()) throw new Error('Management output contains a non-regular file')
      const data = await readFile(resolve(adminBundle, path))
      if (!data.length) throw new Error('Management output contains an empty file')
      files.push({ path, size: data.length, sha256: createHash('sha256').update(data).digest('hex') })
    }
  }
  await inventory()
  await writeFile(resolve(adminBundle, 'asset-manifest.json'), `${JSON.stringify({ version: 1, files })}\n`)
}
const manifest = JSON.parse(await readFile(resolve(bundle, 'public/site/.vite/manifest.json'), 'utf8'))
await cp(resolve(web, 'standalone/favicon.svg'), resolve(bundle, 'public/site/favicon.svg'))
const entry = Object.values(manifest).find(item => item.isEntry)
if (!entry) throw new Error('Missing standalone browser entry')
const styles = Object.values(manifest).filter(item => item.file.endsWith('.css')).map(item => `/site/${item.file}`)
await build({
  configFile: false, root: web, plugins: [vue()],
  resolve: { alias: { '@vue/server-renderer': require.resolve('@vue/server-renderer/dist/server-renderer.esm-bundler.js') } },
  define: { ...definitions, 'import.meta.client': false,
    __SITE_ASSETS__: JSON.stringify({ script: `/site/${entry.file}`, styles }) },
  build: { outDir: bundle, emptyOutDir: false, copyPublicDir: false, minify: true,
    lib: { entry: resolve(web, 'standalone/renderer.ts'), formats: ['iife'], name: 'YujianRenderer', fileName: () => 'renderer.js' },
  },
})
