import assert from 'node:assert/strict'
import test from 'node:test'
import {
  assertCoverageThreshold,
  calculateStatementCoverage,
  coverageArguments,
} from './go-coverage.mjs'

const profile = `mode: atomic
yujian.me/server/a.go:10.1,12.2 8 1
yujian.me/server/a.go:14.1,15.2 2 0
`

test('按 Go profile 的语句数量加权计算覆盖率', () => {
  const result = calculateStatementCoverage(profile)

  assert.deepEqual(result, {
    coveredStatements: 8,
    totalStatements: 10,
    percentage: 80,
  })
})

test('覆盖率达到门槛时返回统计结果', () => {
  assert.equal(assertCoverageThreshold(profile, 80).percentage, 80)
})

test('覆盖率低于门槛时拒绝通过', () => {
  assert.throws(
    () => assertCoverageThreshold(profile, 80.01),
    /Go 语句覆盖率 80\.00% 低于门槛 80\.01%/,
  )
})

test('拒绝没有语句记录的无效 profile', () => {
  assert.throws(
    () => calculateStatementCoverage('mode: atomic\n'),
    /没有可统计的语句/,
  )
})

test('覆盖率运行跨包插桩全部 Go 业务代码', () => {
  assert.ok(coverageArguments('coverage.out').includes('-coverpkg=./...'))
})

test('跨包 profile 对重复源码块只统计一次', () => {
  const duplicated = `mode: atomic
yujian.me/server/a.go:10.1,12.2 8 0
yujian.me/server/a.go:10.1,12.2 8 1
`

  assert.deepEqual(calculateStatementCoverage(duplicated), {
    coveredStatements: 8,
    totalStatements: 8,
    percentage: 100,
  })
})
