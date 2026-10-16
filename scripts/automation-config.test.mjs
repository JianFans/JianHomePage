import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'
import { parse } from 'yaml'

const root = new URL('../', import.meta.url)

async function readText(path) {
  return readFile(new URL(path, root), 'utf8')
}

async function readYaml(path) {
  return parse(await readText(path))
}

test('代码验证工作流覆盖仓库门禁并从清单读取工具版本', async () => {
  const workflow = await readYaml('.github/workflows/code-check.yml')

  assert.deepEqual(Object.keys(workflow.on).sort(), ['pull_request', 'push', 'workflow_dispatch'])
  assert.deepEqual(workflow.permissions, { contents: 'read' })
  assert.equal(workflow.concurrency['cancel-in-progress'], true)

  const requiredJobs = [
    'frontend',
    'go',
    'frontend-coverage',
    'go-coverage',
    'e2e',
    'container',
    'publisher-integration',
  ]
  assert.deepEqual(Object.keys(workflow.jobs).sort(), requiredJobs.sort())

  const steps = Object.values(workflow.jobs).flatMap(job => job.steps)
  const nodeSteps = steps.filter(step => step.uses?.startsWith('actions/setup-node@'))
  const pnpmSteps = steps.filter(step => step.uses?.startsWith('pnpm/action-setup@'))
  const goSteps = steps.filter(step => step.uses?.startsWith('actions/setup-go@'))

  assert.ok(nodeSteps.length > 0)
  assert.ok(nodeSteps.every(step => step.with?.['node-version-file'] === 'package.json'))
  assert.ok(nodeSteps.every(step => !Object.hasOwn(step.with || {}, 'node-version')))
  assert.ok(pnpmSteps.length > 0)
  assert.ok(pnpmSteps.every(step => !Object.hasOwn(step.with || {}, 'version')))
  assert.ok(goSteps.length > 0)
  assert.ok(goSteps.every(step => step.with?.['go-version-file'] === 'apps/server/go.mod'))
  assert.ok(goSteps.every(step => !Object.hasOwn(step.with || {}, 'go-version')))

  const commands = steps.map(step => step.run).filter(Boolean)
  for (const command of [
    'pnpm verify',
    'pnpm verify:go',
    'pnpm test:coverage',
    'pnpm test:coverage:go',
    'pnpm build:server:assets',
    'pnpm test:embedded',
    'pnpm --filter @yujian/web playwright:install',
    'pnpm --filter @yujian/web test:e2e',
    'pnpm --filter @yujian/admin test:e2e',
  ]) {
    assert.ok(commands.includes(command), `工作流缺少命令：${command}`)
  }

  assert.ok(commands.includes('pnpm audit --prod --audit-level high --ignore-unfixable --registry=https://registry.npmjs.org'))
  assert.ok(commands.includes('go test -race ./... -count=1'))
  assert.ok(commands.some(command => command.startsWith('git diff --exit-code -- packages/schema/src/generated.ts')))
  assert.ok(commands.some(command => command.startsWith('git diff --exit-code -- apps/server/internal/contract/schema.json')))

  const containerSteps = workflow.jobs.container.steps
  assert.ok(containerSteps.some(step => step.run === 'docker build --tag yujian-server:ci .'))
  assert.ok(containerSteps.some(step => step.run?.includes('http://127.0.0.1:8080/healthz')))
  assert.ok(containerSteps.some(step => step.uses?.startsWith('aquasecurity/trivy-action@')))
  assert.ok(!containerSteps.some(step => step.run?.includes('yujian-publisher')))
  assert.ok(containerSteps.some(step => step.run?.includes('http://127.0.0.1:8080/preview')))
  assert.ok(containerSteps.some(step => step.run?.includes('--network none') && step.run.includes('--entrypoint /usr/local/bin/yujian-server')))
  const offline = containerSteps.find(step => step.run?.includes('--network none'))
  assert.match(offline.name, /development mode/i)
  assert.doesNotMatch(offline.run, /--rm|sleep 2|State\.Running/)
  assert.match(offline.run, /for attempt in \{1\.\.20\}/)
  assert.match(offline.run, /docker logs yujian-server-offline/)
  assert.match(offline.run, /State\.Status/)
  const integration = workflow.jobs['publisher-integration']
  assert.ok(integration.services.postgres.image.startsWith('postgres:'))
  assert.ok(integration.env.YUJIAN_TEST_POSTGRES_URL)
  assert.ok(integration.steps.some(step => step.run === 'go test -race ./internal/publisher ./internal/store/postgres -count=1'))
  for (const jobName of ['go', 'go-coverage', 'publisher-integration']) {
    const jobSteps = workflow.jobs[jobName].steps
    const install = jobSteps.findIndex(step => step.run === 'pnpm install --frozen-lockfile')
    const assets = jobSteps.findIndex(step => step.run === 'pnpm build:server:assets')
    const verify = jobSteps.findIndex(step => step.run?.startsWith('pnpm verify:go')
      || step.run?.startsWith('pnpm test:coverage:go') || step.run?.startsWith('go test '))
    assert.ok(install >= 0 && install < assets && assets < verify, `${jobName} 必须先生成嵌入资源再验证 Go`)
  }

  const rootPackage = JSON.parse(await readText('package.json'))
  assert.equal(rootPackage.scripts['test:coverage:go'], 'node scripts/go-coverage.mjs --threshold=80')
  assert.match(rootPackage.scripts['verify:go'], /^pnpm check:format:go && /)
  const webPackage = JSON.parse(await readText('apps/web/package.json'))
  assert.equal(webPackage.scripts['playwright:install'], 'playwright install --with-deps chromium')
  const adminPackage = JSON.parse(await readText('apps/admin/package.json'))
  assert.equal(adminPackage.scripts['test:e2e'], 'playwright test')
  assert.equal(adminPackage.devDependencies['@playwright/test'], webPackage.devDependencies['@playwright/test'])
  assert.ok(workflow.jobs.e2e.steps.some(step => step.uses?.startsWith('actions/upload-artifact@') && step.with?.path === 'apps/admin/test-results'))
})

test('Dependabot 覆盖所有依赖生态并按周执行', async () => {
  const config = await readYaml('.github/dependabot.yml')
  const updates = new Map(config.updates.map(update => [
    `${update['package-ecosystem']}:${update.directory}`,
    update,
  ]))

  for (const key of ['npm:/', 'gomod:/apps/server', 'docker:/', 'github-actions:/']) {
    assert.ok(updates.has(key), `Dependabot 缺少 ${key}`)
    assert.equal(updates.get(key).schedule.interval, 'weekly')
  }

  assert.deepEqual(updates.get('npm:/').ignore ?? [], [], '依赖升级必须通过兼容性验证，不能忽略新版')

  assert.ok(!updates.has('docker:/apps/publisher'), '独立 Node 发布镜像已停用')
})

test('单程序镜像只在编译阶段使用 Node 并嵌入管理页面和渲染程序', async () => {
  const dockerfile = await readText('Dockerfile')
  const rootPackage = JSON.parse(await readText('package.json'))
  assert.match(rootPackage.engines.node, /^>=\d+\.\d+\.\d+$/, 'Node 只声明最低版本，不限制后续主版本')
  const image = dockerfile.match(/^FROM node:(\d+)(?:\.\d+\.\d+)?-[^\s]+ AS assets$/m)
  assert.ok(image, '资源编译镜像必须声明明确的 Node 版本标签')
  assert.match(dockerfile, /^FROM node:24-bookworm-slim AS assets$/m)
  assert.ok(Number(image[1]) >= Number(rootPackage.engines.node.match(/\d+/)[0]))
  assert.match(dockerfile, /npm install --global --prefix \/opt\/pnpm/)
  assert.ok(dockerfile.includes('require(\'./package.json\').packageManager'))
  assert.doesNotMatch(dockerfile, /\bcorepack\b/)
  assert.match(dockerfile, /pnpm install --frozen-lockfile/)
  assert.match(dockerfile, /pnpm build:server:assets/)
  assert.match(dockerfile, /COPY --from=assets \/workspace\/apps\/server\/internal\/webassets\/bundle\/ \.\/internal\/webassets\/bundle\//)
  assert.ok(dockerfile.indexOf('pnpm build:server:assets') < dockerfile.indexOf('go build'))
  const runtime = dockerfile.slice(dockerfile.lastIndexOf('FROM '))
  assert.doesNotMatch(runtime, /\bnode\b|\bpnpm\b|node_modules|\/workspace/)
  assert.doesNotMatch(dockerfile, /PUBLISHER_CODE_REVISION|ARG CODE_REVISION/)
})

test('Go API 镜像采用多阶段非 root 构建并排除无关文件', async () => {
  const dockerfile = await readText('Dockerfile')
  const dockerignore = await readText('.dockerignore')

  assert.match(dockerfile, /^FROM golang:[^\s]+ AS build$/m)
  assert.match(dockerfile, /^FROM gcr\.io\/distroless\/static-debian12:nonroot$/m)
  assert.match(dockerfile, /go build[^\n]+\.\/cmd\/api/)
  assert.match(dockerfile, /^USER nonroot:nonroot$/m)
  assert.match(dockerfile, /^EXPOSE 8080$/m)
  assert.match(dockerfile, /^ENTRYPOINT \["\/usr\/local\/bin\/yujian-server"\]$/m)

  for (const ignored of ['.git', 'node_modules', 'coverage', 'apps/web/.output']) {
    assert.match(dockerignore, new RegExp(`^${ignored.replaceAll('.', '\\.')}/?$`, 'm'))
  }
  for (const required of ['apps/web', 'apps/admin', 'packages', 'scripts', 'content']) {
    assert.ok(!dockerignore.split(/\r?\n/).includes(required), `Docker 资源编译需要 ${required}`)
  }
  for (const ignored of ['**/node_modules', '**/.env', '**/.env.*', 'apps/server/internal/webassets/bundle']) {
    assert.ok(dockerignore.split(/\r?\n/).includes(ignored), `Docker 必须排除 ${ignored}`)
  }
})
