import { resolve } from 'node:path'
import { tmpdir } from 'node:os'
import { fileURLToPath } from 'node:url'

/** Reject absent or ambiguous configuration without including secret values. */
function required(env, name) {
  const value = env[name]
  if (typeof value !== 'string' || !value || value.trim() !== value || /[\r\n\0]/.test(value)) {
    throw new Error(`${name} is required and must not contain surrounding whitespace or control characters`)
  }
  return value
}

/** Restrict service URLs to credential-free HTTPS with no query or fragment. */
function httpsURL(value, name) {
  const url = new URL(value)
  if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) {
    throw new Error(`${name} must be a credential-free HTTPS URL`)
  }
  return url.href
}

/** Parse bounded integers exactly, rather than accepting partial parseInt results. */
function integer(value, fallback, minimum, maximum, name) {
  if (value === undefined) return fallback
  if (!/^\d+$/.test(value) || Number(value) < minimum || Number(value) > maximum) {
    throw new Error(`${name} is outside its allowed range`)
  }
  return Number(value)
}

/** Load an explicit production-only publisher configuration; there are no development providers. */
export function loadConfig(env = process.env) {
  const databaseURL = required(env, 'PUBLISHER_DATABASE_URL')
  if (!['postgres:', 'postgresql:'].includes(new URL(databaseURL).protocol)) throw new Error('PostgreSQL is required')
  const token = required(env, 'PUBLISHER_TOKEN')
  const pagesToken = required(env, 'PAGES_API_TOKEN')
  if (token.length < 32 || /\s/.test(token) || token === pagesToken) throw new Error('Use distinct, strong publisher and Pages tokens')
  const projectId = required(env, 'PAGES_PROJECT_ID')
  if (!/^[a-zA-Z0-9_-]{1,128}$/.test(projectId)) throw new Error('PAGES_PROJECT_ID is invalid')
  const region = required(env, 'PAGES_REGION')
  if (!['china', 'global'].includes(region)) throw new Error('PAGES_REGION must be china or global')
  const codeRevision = required(env, 'PUBLISHER_CODE_REVISION')
  if (!/^[a-f0-9]{40}$/.test(codeRevision)) throw new Error('PUBLISHER_CODE_REVISION must be a full Git commit')
  if (env.S3_USE_PATH_STYLE !== undefined && !['true', 'false'].includes(env.S3_USE_PATH_STYLE)) throw new Error('S3_USE_PATH_STYLE must be true or false')
  const sourceRoot = resolve(env.PUBLISHER_SOURCE_ROOT || fileURLToPath(new URL('../../..', import.meta.url)))
  return {
    databaseURL, token, pagesToken, sourceRoot,
    workRoot: resolve(env.PUBLISHER_WORK_ROOT || resolve(tmpdir(), 'yujian-publisher')),
    host: env.PUBLISHER_HOST || '127.0.0.1',
    port: integer(env.PUBLISHER_PORT, 8091, 1, 65535, 'PUBLISHER_PORT'),
    buildTimeoutMs: integer(env.PUBLISHER_BUILD_TIMEOUT_MS, 600000, 10000, 1800000, 'PUBLISHER_BUILD_TIMEOUT_MS'),
    target: { projectId, region, codeRevision, publicUrl: httpsURL(required(env, 'PAGES_PUBLIC_URL'), 'PAGES_PUBLIC_URL') },
    s3: {
      endpoint: httpsURL(required(env, 'S3_ENDPOINT'), 'S3_ENDPOINT'),
      region: required(env, 'S3_REGION'), bucket: required(env, 'S3_BUCKET'),
      credentials: {
        accessKeyId: required(env, 'S3_ACCESS_KEY_ID'), secretAccessKey: required(env, 'S3_SECRET_ACCESS_KEY'),
        ...(env.S3_SESSION_TOKEN ? { sessionToken: required(env, 'S3_SESSION_TOKEN') } : {}),
      },
      forcePathStyle: env.S3_USE_PATH_STYLE === 'true',
    },
  }
}
