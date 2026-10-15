import { spawn } from 'node:child_process'
import { join } from 'node:path'

const MAX_OUTPUT_BYTES = 1024 * 1024

/**
 * Run a shell-free build; bound execution and stop the whole tree on every abnormal exit.
 * @param {{command: string, args: string[], cwd: string, env: Record<string,string>, timeoutMs: number}} options
 * @returns {Promise<void>}
 */
export function runBuildCommand({ command, args, cwd, env, timeoutMs }) {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd, env, shell: false, windowsHide: true, detached: process.platform !== 'win32', stdio: ['ignore', 'pipe', 'pipe'],
    })
    let failed = false
    let size = 0
    let termination
    const stop = () => {
      failed = true
      if (termination) return termination
      termination = new Promise((done) => {
        if (!child.pid) { done(); return }
        const fallback = () => { try { child.kill('SIGKILL') } catch { /* Process already gone. */ } }
        if (process.platform === 'win32' && (env.SystemRoot || env.SYSTEMROOT || process.env.SystemRoot)) {
          const systemRoot = env.SystemRoot || env.SYSTEMROOT || process.env.SystemRoot
          const killer = spawn(join(systemRoot, 'System32', 'taskkill.exe'), ['/pid', String(child.pid), '/T', '/F'], {
            env, windowsHide: true, shell: false, stdio: 'ignore',
          })
          const timeout = setTimeout(() => { killer.kill('SIGKILL'); fallback(); done() }, 5000)
          const finish = (code) => { clearTimeout(timeout); if (code !== 0) fallback(); done() }
          killer.on('error', () => finish(1))
          killer.on('close', finish)
        } else {
          try { process.kill(-child.pid, 'SIGKILL') } catch { fallback() }
          done()
        }
      })
      return termination
    }
    const timeout = setTimeout(stop, timeoutMs)
    const countOutput = (chunk) => { size += chunk.length; if (size > MAX_OUTPUT_BYTES) stop() }
    child.stdout.on('data', countOutput)
    child.stderr.on('data', countOutput)
    child.on('error', async () => {
      clearTimeout(timeout)
      await stop()
      reject(new Error('Build process failed'))
    })
    child.on('exit', (code) => { if (code !== 0) stop() })
    child.on('close', async (code) => {
      clearTimeout(timeout)
      if (failed || code !== 0) { await stop(); reject(new Error('Build process failed')) }
      else resolve()
    })
  })
}
