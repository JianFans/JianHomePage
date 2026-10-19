import { execFileSync } from 'node:child_process'
import { fileURLToPath, pathToFileURL } from 'node:url'

/** Compare numeric version components without lexicographic ordering or precision loss. */
function compare(left, right) {
  for (let index = 0; index < 3; index++) {
    if (left[index] !== right[index]) return left[index] > right[index] ? 1 : -1
  }
  return 0
}

/** Increment the highest stable vX.Y.Z tag; a repository without releases starts at 0.0.0. */
export function nextVersion(tags, bump = 'patch') {
  if (!['patch', 'minor', 'major'].includes(bump)) throw new Error('bump 必须为 patch、minor 或 major')
  const versions = [[0n, 0n, 0n]]
  for (const tag of tags) {
    const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(tag)
    if (match?.[0] === tag) versions.push(match.slice(1).map(BigInt))
  }
  versions.sort(compare)
  const version = [...versions.at(-1)]
  const index = { major: 0, minor: 1, patch: 2 }[bump]
  version[index]++
  for (let lower = index + 1; lower < 3; lower++) version[lower] = 0n
  return version.join('.')
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const tags = execFileSync('git', ['tag', '--list'], {
    cwd: fileURLToPath(new URL('..', import.meta.url)), encoding: 'utf8',
  }).split(/\r?\n/).filter(Boolean)
  console.log(nextVersion(tags, process.argv[2]))
}
