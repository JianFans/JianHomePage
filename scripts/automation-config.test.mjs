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
    'pnpm --filter @yujian/web playwright:install',
    'pnpm --filter @yujian/web test:e2e',
  ]) {
    assert.ok(commands.includes(command), `工作流缺少命令：${command}`)
  }

  assert.ok(commands.includes('pnpm audit --prod --audit-level high --registry=https://registry.npmjs.org'))
  assert.ok(commands.includes('go test -race ./... -count=1'))
  assert.ok(commands.some(command => command.startsWith('git diff --exit-code -- packages/schema/src/generated.ts')))
  assert.ok(commands.some(command => command.startsWith('git diff --exit-code -- apps/server/internal/contract/schema.json')))

  const containerSteps = workflow.jobs.container.steps
  assert.ok(containerSteps.some(step => step.run === 'docker build --tag yujian-server:ci .'))
  assert.ok(containerSteps.some(step => step.run?.includes('http://127.0.0.1:8080/healthz')))
  assert.ok(containerSteps.some(step => step.uses?.startsWith('aquasecurity/trivy-action@')))

  const rootPackage = JSON.parse(await readText('package.json'))
  assert.equal(rootPackage.scripts['test:coverage:go'], 'node scripts/go-coverage.mjs --threshold=80')
  assert.match(rootPackage.scripts['verify:go'], /^pnpm check:format:go && /)
  const webPackage = JSON.parse(await readText('apps/web/package.json'))
  assert.equal(webPackage.scripts['playwright:install'], 'playwright install --with-deps chromium')
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
})
