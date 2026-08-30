import assert from 'node:assert/strict'
import test from 'node:test'
import {
  assertNoUnformattedFiles,
  normalizeLineEndings,
  parseTrackedGoFiles,
} from './check-go-format.mjs'

test('解析 Git 跟踪的 Go 文件并忽略空行', () => {
  assert.deepEqual(
    parseTrackedGoFiles('apps/server/main.go\r\n\napps/server/internal/api.go\n'),
    ['apps/server/main.go', 'apps/server/internal/api.go'],
  )
})

test('发现未格式化文件时拒绝通过', () => {
  assert.throws(
    () => assertNoUnformattedFiles('apps/server/main.go\n'),
    /Go 文件未通过 gofmt：apps\/server\/main\.go/,
  )
})

test('没有未格式化文件时通过', () => {
  assert.doesNotThrow(() => assertNoUnformattedFiles(''))
})

test('比较 gofmt 输出前统一 Windows 行尾', () => {
  assert.equal(normalizeLineEndings('package main\r\n\r\n'), 'package main\n\n')
})
