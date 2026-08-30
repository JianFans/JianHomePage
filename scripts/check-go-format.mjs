import { spawnSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

export function parseTrackedGoFiles(output) {
  return output.split(/\r?\n/).map(file => file.trim()).filter(Boolean)
}

export function normalizeLineEndings(source) {
  return source.replaceAll('\r\n', '\n')
}

export function assertNoUnformattedFiles(output) {
  const files = parseTrackedGoFiles(output)
  if (files.length > 0) {
    throw new Error(`Go 文件未通过 gofmt：${files.join(', ')}`)
  }
}

function execute(command, args, cwd) {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8' })
  if (result.error) throw result.error
  if (result.status !== 0) {
    throw new Error(`${command} 执行失败：${result.stderr.trim() || `exit ${result.status}`}`)
  }
  return result.stdout
}

function findUnformattedFiles(files, cwd) {
  return files.filter((file) => {
    const source = normalizeLineEndings(readFileSync(resolve(cwd, file), 'utf8'))
    const result = spawnSync('gofmt', [], { cwd, input: source, encoding: 'utf8' })
    if (result.error) throw result.error
    if (result.status !== 0) {
      throw new Error(`gofmt 执行失败：${result.stderr.trim() || `exit ${result.status}`}`)
    }
    return result.stdout !== source
  })
}

function run() {
  const workspaceRoot = fileURLToPath(new URL('../', import.meta.url))
  const files = parseTrackedGoFiles(execute(
    'git',
    ['ls-files', '--cached', '--others', '--exclude-standard', '--', ':(glob)apps/server/**/*.go'],
    workspaceRoot,
  ))
  if (files.length === 0) {
    throw new Error('没有找到 Git 跟踪的 Go 文件')
  }

  assertNoUnformattedFiles(findUnformattedFiles(files, workspaceRoot).join('\n'))
  console.log(`Go 格式校验通过：${files.length} 个文件`)
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  run()
}
