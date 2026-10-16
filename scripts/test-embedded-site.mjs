// Browser validation for the output of the real Go executable. Node is only the
// developer-side test runner; the executable receives an empty executable PATH.
import { createServer } from 'node:http'
import { spawn, spawnSync } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, readFile, rm, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { resolve, extname, sep } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createRequire } from 'node:module'

const root = fileURLToPath(new URL('..', import.meta.url))
const temporary = await mkdtemp(resolve(tmpdir(), 'yujian-embedded-site-'))
const executable = resolve(temporary, process.platform === 'win32' ? 'server.exe' : 'server')
const directory = resolve(temporary, 'site')
const mediaRoot = resolve(root, 'apps/web/public')
const mime = { '.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.webp': 'image/webp', '.svg': 'image/svg+xml', '.mp3': 'audio/mpeg', '.wav': 'audio/wav', '.mp4': 'video/mp4', '.xml': 'application/xml', '.txt': 'text/plain' }
const server = createServer(async (request, response) => {
  try {
    const path = decodeURIComponent(new URL(request.url, 'http://localhost').pathname)
    const base = path.startsWith('/media/') ? mediaRoot : directory
    const file = resolve(base, `.${path === '/' ? '/index.html' : path}`)
    if (!file.startsWith(base + sep) || !(await stat(file)).isFile()) throw new Error('Not found')
    response.setHeader('Content-Type', mime[extname(file)] || 'application/octet-stream')
    response.end(await readFile(file))
  } catch { response.writeHead(404); response.end() }
})

try {
  const build = spawnSync('go', ['build', '-o', executable, './cmd/api'], { cwd: resolve(root, 'apps/server'), stdio: 'inherit' })
  if (build.error) throw build.error
  if (build.status !== 0) throw new Error('Go build failed')
  const exported = spawnSync(executable, ['--export-site', resolve(root, 'content/fixtures/homepage.json'), '--output', directory], {
    cwd: temporary, env: { ...process.env, PATH: '', Path: '' }, stdio: 'inherit',
  })
  if (exported.error) throw exported.error
  if (exported.status !== 0) throw new Error('Standalone Go rendering failed')
  server.listen(0, '127.0.0.1')
  await once(server, 'listening')
  const require = createRequire(resolve(root, 'apps/web/package.json'))
  const runner = spawn(process.execPath, [require.resolve('@playwright/test/cli'), 'test'], {
    cwd: resolve(root, 'apps/web'), stdio: 'inherit', env: { ...process.env, YUJIAN_EMBEDDED_SITE: '1', PLAYWRIGHT_BASE_URL: `http://127.0.0.1:${server.address().port}` },
  })
  const [code] = await once(runner, 'exit')
  if (code !== 0) throw new Error('Embedded site browser checks failed')
} finally {
  server.closeAllConnections()
  await new Promise(resolveClose => server.close(resolveClose))
  await rm(temporary, { recursive: true, force: true })
}
