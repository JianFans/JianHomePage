import assert from 'node:assert/strict'
import { test } from 'node:test'

const module = await import('../src/config.mjs').catch(() => ({}))
const env = {
  PUBLISHER_DATABASE_URL: 'postgresql://localhost/yujian', PUBLISHER_TOKEN: 'g'.repeat(40),
  PAGES_API_TOKEN: 'pages-token', PAGES_PROJECT_ID: 'project', PAGES_REGION: 'china',
  PAGES_PUBLIC_URL: 'https://yujian.me', PUBLISHER_CODE_REVISION: 'a'.repeat(40),
  S3_ENDPOINT: 'https://cos.ap-singapore.myqcloud.com', S3_REGION: 'ap-singapore',
  S3_BUCKET: 'snapshots', S3_ACCESS_KEY_ID: 'id', S3_SECRET_ACCESS_KEY: 'key',
}

test('production publisher requires explicit provider and source configuration', () => {
  assert.equal(typeof module.loadConfig, 'function', 'publisher configuration must exist')
  const config = module.loadConfig(env)
  assert.equal(config.target.region, 'china')
  assert.equal(config.target.publicUrl, 'https://yujian.me/')
  assert.equal(config.port, 8091)
  for (const key of Object.keys(env)) {
    assert.throws(() => module.loadConfig({ ...env, [key]: '' }), undefined, key)
  }
})

test('invalid origins, ports, revisions, storage options and reused tokens fail closed', () => {
  for (const invalid of [
    { PAGES_REGION: 'auto' }, { PAGES_PUBLIC_URL: 'http://yujian.me' },
    { PAGES_PUBLIC_URL: 'https://user:pass@yujian.me' }, { PAGES_PUBLIC_URL: 'https://yujian.me/?secret=x' },
    { PUBLISHER_TOKEN: 'short' }, { PAGES_API_TOKEN: env.PUBLISHER_TOKEN },
    { PUBLISHER_PORT: '0' }, { PUBLISHER_PORT: '12x' }, { PUBLISHER_CODE_REVISION: 'main' },
    { S3_USE_PATH_STYLE: 'maybe' }, { S3_ENDPOINT: 'http://localhost' },
    { PUBLISHER_BUILD_TIMEOUT_MS: '1' }, { PUBLISHER_DATABASE_URL: 'https://example.com' },
  ]) {
    assert.throws(() => module.loadConfig({ ...env, ...invalid }))
  }
})
