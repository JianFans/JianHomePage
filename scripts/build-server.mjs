import { spawnSync } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('..', import.meta.url))
const assets = spawnSync(process.execPath, [resolve(root, 'scripts/build-server-assets.mjs')], { cwd: root, stdio: 'inherit' })
if (assets.error) throw assets.error
if (assets.status !== 0) throw new Error('Embedded asset build failed')
await mkdir(resolve(root, 'dist'), { recursive: true })
const target = process.env.GOOS || process.platform
const output = resolve(root, 'dist', target === 'windows' || target === 'win32' ? 'yujian-server.exe' : 'yujian-server')
const result = spawnSync('go', ['build', '-trimpath', '-ldflags=-s -w', '-o', output, './cmd/api'], {
  cwd: resolve(root, 'apps/server'), stdio: 'inherit', env: { ...process.env, CGO_ENABLED: '0' },
})
if (result.error) throw result.error
if (result.status !== 0) throw new Error('Go server build failed')
console.log(`Single executable: ${output}`)
