import { once } from 'node:events'
import { setTimeout as delay } from 'node:timers/promises'
import { Makers } from '@edgeone/makers-sdk'
import { S3Client } from '@aws-sdk/client-s3'
import pg from 'pg'
import { createBuilder } from './builder.mjs'
import { loadConfig } from './config.mjs'
import { createGateway } from './http.mjs'
import { createPagesProvider } from './pages.mjs'
import { createSnapshotReader } from './snapshot-reader.mjs'
import { verifySourceRevision } from './source.mjs'
import { createStore } from './store.mjs'
import { createWorker } from './worker.mjs'

/** Run the private publishing gateway; dependencies and credentials never enter the build environment. */
async function main() {
  const config = loadConfig()
  await verifySourceRevision(config.sourceRoot, config.target.codeRevision)
  const pool = new pg.Pool({ connectionString: config.databaseURL, max: 5, connectionTimeoutMillis: 10000, query_timeout: 15000, keepAlive: true })
  pool.on('error', () => console.error('Publisher database connection unavailable'))
  const store = createStore(pool, { onLockLost: () => {
    console.error('Publisher database lock lost; shutting down')
    process.exit(1)
  } })
  try { await store.health() } catch { await pool.end(); throw new Error('Publisher migration or database unavailable') }
  const s3 = new S3Client(config.s3)
  const pages = createPagesProvider(new Makers({ token: config.pagesToken, region: config.target.region, retries: 0, timeout: 60 }))
  const build = createBuilder({ sourceRoot: config.sourceRoot, workRoot: config.workRoot, readSnapshot: createSnapshotReader(s3, config.s3.bucket), timeoutMs: config.buildTimeoutMs })
  const workerOptions = { pages, build, target: config.target }
  const gateway = createGateway({
    store, token: config.token, target: config.target,
    reconcile: (id, input) => store.withLock(locked => createWorker({ ...workerOptions, store: locked }).reconcile(id, input)),
  })
  let running = null
  let stopping = false
  /** Avoid queuing overlapping ticks while a long build owns the database lock. */
  function tick() {
    if (running || stopping) return
    running = store.withLock(locked => createWorker({ ...workerOptions, store: locked }).tick())
      .catch(() => console.error('Publisher reconciliation unavailable; will retry'))
      .finally(() => { running = null })
  }
  gateway.listen(config.port, config.host)
  await once(gateway, 'listening')
  const timer = setInterval(tick, 5000)
  tick()
  console.info('Publisher ready')
  /** Preserve durable phase state while allowing in-flight work a bounded shutdown window. */
  async function shutdown() {
    if (stopping) return
    stopping = true
    clearInterval(timer)
    gateway.closeAllConnections()
    gateway.close()
    const timeout = delay(30000).then(() => process.exit(1))
    await running
    s3.destroy()
    await pool.end()
    void timeout
    process.exit(0)
  }
  process.on('SIGINT', shutdown)
  process.on('SIGTERM', shutdown)
}

main().catch(() => { console.error('Publisher startup failed; check configuration, source revision and migrations'); process.exit(1) })
