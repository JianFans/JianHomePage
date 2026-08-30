import { spawnSync } from 'node:child_process'
import { mkdir, readFile } from 'node:fs/promises'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

export const DEFAULT_GO_COVERAGE_THRESHOLD = 80

export function calculateStatementCoverage(profile) {
  const blocks = new Map()
  let coveredStatements = 0
  let totalStatements = 0

  for (const line of profile.split(/\r?\n/).slice(1)) {
    if (!line.trim()) continue
    const fields = line.trim().split(/\s+/)
    if (fields.length !== 3) {
      throw new Error(`Go coverage profile 记录无效：${line}`)
    }
    const statementCount = Number(fields[1])
    const executionCount = Number(fields[2])
    if (!Number.isSafeInteger(statementCount) || statementCount < 0 || !Number.isSafeInteger(executionCount) || executionCount < 0) {
      throw new Error(`Go coverage profile 记录无效：${line}`)
    }
    const blockKey = fields[0]
    const existing = blocks.get(blockKey)
    if (existing && existing.statementCount !== statementCount) {
      throw new Error(`Go coverage profile 重复区块语句数不一致：${line}`)
    }
    blocks.set(blockKey, {
      statementCount,
      covered: Boolean(existing?.covered || executionCount > 0),
    })
  }

  for (const block of blocks.values()) {
    totalStatements += block.statementCount
    if (block.covered) coveredStatements += block.statementCount
  }

  if (totalStatements === 0) {
    throw new Error('Go coverage profile 没有可统计的语句')
  }

  return {
    coveredStatements,
    totalStatements,
    percentage: coveredStatements / totalStatements * 100,
  }
}

export function assertCoverageThreshold(profile, threshold = DEFAULT_GO_COVERAGE_THRESHOLD) {
  if (!Number.isFinite(threshold) || threshold < 0 || threshold > 100) {
    throw new Error(`Go 覆盖率门槛无效：${threshold}`)
  }
  const result = calculateStatementCoverage(profile)
  if (result.percentage < threshold) {
    throw new Error(`Go 语句覆盖率 ${result.percentage.toFixed(2)}% 低于门槛 ${threshold.toFixed(2)}%`)
  }
  return result
}

export function coverageArguments(profilePath) {
  return [
    'test',
    '-coverpkg=./...',
    '-covermode=atomic',
    `-coverprofile=${profilePath}`,
    './...',
  ]
}

async function run() {
  const workspaceRoot = fileURLToPath(new URL('../', import.meta.url))
  const serverRoot = resolve(workspaceRoot, 'apps/server')
  const profilePath = resolve(workspaceRoot, 'coverage/go.coverprofile')
  const thresholdArgument = process.argv.find(argument => argument.startsWith('--threshold='))
  const threshold = thresholdArgument
    ? Number(thresholdArgument.slice('--threshold='.length))
    : DEFAULT_GO_COVERAGE_THRESHOLD

  await mkdir(dirname(profilePath), { recursive: true })
  const result = spawnSync('go', coverageArguments(profilePath), {
    cwd: serverRoot,
    stdio: 'inherit',
  })
  if (result.error) throw result.error
  if (result.status !== 0) process.exit(result.status ?? 1)

  const coverage = assertCoverageThreshold(await readFile(profilePath, 'utf8'), threshold)
  console.log(`Go 语句覆盖率：${coverage.percentage.toFixed(2)}%（门槛 ${threshold.toFixed(2)}%）`)
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  await run()
}
