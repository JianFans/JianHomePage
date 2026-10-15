import { createHash, timingSafeEqual } from 'node:crypto'
import { createServer } from 'node:http'
import { buildResponse } from './worker.mjs'

/** HTTP errors carry only fixed, safe messages and a response status. */
class RequestError extends Error {
  /** Keep client-visible failures distinct from redacted infrastructure exceptions. */
  constructor(status, message) { super(message); this.status = status }
}

/** Compare fixed-size digests so missing and mismatched credentials do not leak token length. */
function authorized(request, token) {
  const supplied = request.headers.authorization || ''
  if (request.headersDistinct.authorization?.length !== 1) return false
  return timingSafeEqual(createHash('sha256').update(supplied).digest(), createHash('sha256').update(`Bearer ${token}`).digest())
}

/** Limit memory before parsing; continue draining oversized input so a 413 response can be delivered. */
async function readJSON(request) {
  if (request.headersDistinct['content-type']?.length !== 1 || !/^application\/json(?:\s*;\s*charset=utf-8)?$/i.test(request.headers['content-type'] || '')) {
    throw new RequestError(415, 'JSON content type required')
  }
  const body = await new Promise((resolve, reject) => {
    const chunks = []
    let size = 0
    let exceeded = false
    request.on('data', chunk => {
      size += chunk.length
      if (size > 16384) {
        exceeded = true
        chunks.length = 0
        reject(new RequestError(413, 'Request body too large'))
      } else if (!exceeded) chunks.push(chunk)
    })
    request.on('end', () => { if (!exceeded) resolve(Buffer.concat(chunks).toString('utf8')) })
    request.on('error', reject)
    request.on('aborted', () => reject(new RequestError(400, 'Request aborted')))
  })
  try {
    const value = JSON.parse(body)
    if (!value || typeof value !== 'object' || Array.isArray(value)) throw new Error('not an object')
    return value
  } catch { throw new RequestError(400, 'Invalid JSON object') }
}

/** Accept only the immutable object reference emitted by the Go publishing domain. */
function validateBuild(body) {
  if (Object.keys(body).some(key => !['releaseId', 'snapshotKey', 'snapshotChecksum'].includes(key))
    || typeof body.releaseId !== 'string' || !/^rel_[a-z0-9_]+$/.test(body.releaseId)
    || typeof body.snapshotChecksum !== 'string' || !/^sha256:[a-f0-9]{64}$/.test(body.snapshotChecksum)
    || body.snapshotKey !== `snapshots/${body.releaseId}/${body.snapshotChecksum}.json`) {
    throw new RequestError(400, 'Invalid immutable snapshot reference')
  }
  return { releaseId: body.releaseId, snapshotKey: body.snapshotKey, snapshotChecksum: body.snapshotChecksum }
}

/** Recover Go's UTF-8 header bytes from Node's Latin-1 view; preserve BOM and match strings.TrimSpace. */
function normalizeIdempotencyKey(value) {
  let key
  try {
    key = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true })
      .decode(Buffer.from(value, 'latin1'))
      .replace(/^\p{White_Space}+|\p{White_Space}+$/gu, '')
  } catch { throw new RequestError(400, 'Invalid Idempotency-Key') }
  if (Buffer.byteLength(key) < 8 || Buffer.byteLength(key) > 128 || /\p{Cc}/u.test(key)) {
    throw new RequestError(400, 'Invalid Idempotency-Key')
  }
  return key
}

/** Serve the stable BuildTrigger contract; long builds run outside the request lifecycle. */
export function createGateway({ store, token, target, reconcile }) {
  return createServer({ requestTimeout: 15000, headersTimeout: 10000, maxHeaderSize: 8192 }, async (request, response) => {
    response.setHeader('Cache-Control', 'no-store')
    response.setHeader('Content-Type', 'application/json; charset=utf-8')
    request.setTimeout(15000, () => request.destroy())
    try {
      const path = request.url
      if (path === '/healthz' && request.method === 'GET') {
        await store.health()
        response.end(JSON.stringify({ status: 'ok' }))
        return
      }
      if (!authorized(request, token)) throw new RequestError(401, 'Unauthorized')
      const match = /^\/builds\/(build_[a-f0-9]{64})(\/reconcile)?$/.exec(path)
      if (path === '/builds') {
        if (request.method !== 'POST') throw new RequestError(405, 'Method not allowed')
        if (request.headersDistinct['idempotency-key']?.length !== 1) {
          throw new RequestError(400, 'Invalid Idempotency-Key')
        }
        const key = normalizeIdempotencyKey(request.headers['idempotency-key'])
        const body = validateBuild(await readJSON(request))
        const job = await store.enqueue(key, body, target)
        response.statusCode = 202
        response.end(JSON.stringify(buildResponse(job)))
        return
      }
      if (!match) throw new RequestError(404, 'Not found')
      if (match[2]) {
        if (request.method !== 'POST') throw new RequestError(405, 'Method not allowed')
        const body = await readJSON(request)
        if (Object.keys(body).some(key => !['deploymentId', 'confirmedAbsent', 'note'].includes(key))) throw new RequestError(400, 'Unknown reconciliation field')
        const job = await reconcile(match[1], body)
        if (!job) throw new RequestError(409, 'Publisher busy; retry after the current operation')
        response.end(JSON.stringify(buildResponse(job)))
        return
      }
      if (request.method !== 'GET') throw new RequestError(405, 'Method not allowed')
      const job = await store.get(match[1])
      if (!job) throw new RequestError(404, 'Build not found')
      response.end(JSON.stringify(buildResponse(job)))
    } catch (error) {
      const status = error instanceof RequestError ? error.status : error?.name === 'ConflictError' ? 409 : 503
      response.statusCode = status
      response.end(JSON.stringify({ error: error instanceof RequestError ? error.message : status === 409 ? 'Publishing conflict' : 'Publisher unavailable' }))
      request.resume()
    }
  })
}
