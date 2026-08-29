import { access, readFile, readdir, stat } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import {
  extname,
  isAbsolute,
  relative,
  resolve,
} from 'node:path'

export const DEFAULT_INITIAL_SCRIPT_BUDGET = 320 * 1024

const requiredFiles = ['index.html', 'robots.txt', 'sitemap.xml']
const runtimeContentApiPattern = /(?:\/api\/(?:content|homepage|snapshot)\b|CONTENT_(?:API|ENDPOINT|SNAPSHOT_URL)|runtime-content-api)/i
const textExtensions = new Set(['.html', '.js', '.mjs', '.json'])

function parseAttributes(source) {
  const attributes = new Map()
  const pattern = /([\w:-]+)\s*=\s*(["'])(.*?)\2/g
  for (const match of source.matchAll(pattern)) {
    attributes.set(match[1].toLowerCase(), match[3])
  }
  return attributes
}

function localReference(value) {
  if (
    !value
    || value.startsWith('#')
    || value.startsWith('//')
    || /^(?:data|mailto|tel|javascript):/i.test(value)
    || /^[a-z][a-z\d+.-]*:/i.test(value)
  ) {
    return null
  }

  const path = value.split(/[?#]/, 1)[0]
  if (!path || path === '/') {
    return null
  }
  return decodeURIComponent(path).replace(/^\/+/, '')
}

function extractPageReferences(html) {
  const references = new Set()
  const initialScripts = new Set()
  const tagPattern = /<(script|link|img|source|video|audio)\b([^>]*)>/gi

  for (const match of html.matchAll(tagPattern)) {
    const tagName = match[1].toLowerCase()
    const attributes = parseAttributes(match[2])
    const rel = (attributes.get('rel') || '').toLowerCase().split(/\s+/)
    const candidates = []

    if (tagName === 'script') {
      candidates.push(attributes.get('src'))
    } else if (tagName === 'link') {
      if (rel.some(value => ['stylesheet', 'modulepreload', 'preload', 'icon'].includes(value))) {
        candidates.push(attributes.get('href'))
      }
    } else {
      candidates.push(attributes.get('src'), attributes.get('poster'))
      const srcset = attributes.get('srcset')
      if (srcset) {
        candidates.push(...srcset.split(',').map(value => value.trim().split(/\s+/, 1)[0]))
      }
    }

    for (const candidate of candidates) {
      const reference = localReference(candidate)
      if (!reference) {
        continue
      }
      references.add(reference)
      if (
        tagName === 'script'
        || (tagName === 'link' && (
          rel.includes('modulepreload')
          || (rel.includes('preload') && attributes.get('as') === 'script')
        ))
      ) {
        initialScripts.add(reference)
      }
    }
  }

  return { initialScripts, references }
}

function resolveInside(root, reference) {
  const target = resolve(root, reference)
  const relativePath = relative(root, target)
  if (relativePath.startsWith('..') || isAbsolute(relativePath)) {
    throw new Error(`静态资源路径越界：${reference}`)
  }
  return target
}

async function requireFile(root, name) {
  const target = resolveInside(root, name)
  try {
    const details = await stat(target)
    if (!details.isFile()) {
      throw new Error('not-file')
    }
  } catch {
    throw new Error(`静态产物缺少必需文件：${name}`)
  }
  return target
}

async function collectTextFiles(directory) {
  const files = []
  for (const entry of await readdir(directory, { withFileTypes: true })) {
    const target = resolve(directory, entry.name)
    if (entry.isDirectory()) {
      files.push(...await collectTextFiles(target))
    } else if (entry.isFile() && textExtensions.has(extname(entry.name))) {
      files.push(target)
    }
  }
  return files
}

export async function verifyStaticOutput(
  outputDirectory,
  { initialScriptBudget = DEFAULT_INITIAL_SCRIPT_BUDGET } = {},
) {
  const root = resolve(outputDirectory)
  for (const name of requiredFiles) {
    await requireFile(root, name)
  }

  const indexPath = resolveInside(root, 'index.html')
  const html = await readFile(indexPath, 'utf8')
  const { initialScripts, references } = extractPageReferences(html)

  for (const reference of references) {
    try {
      await access(resolveInside(root, reference))
    } catch {
      throw new Error(`首页引用的静态资源不存在：${reference}`)
    }
  }

  for (const file of await collectTextFiles(root)) {
    const content = await readFile(file, 'utf8')
    if (runtimeContentApiPattern.test(content)) {
      throw new Error(`检测到运行时内容 API 标记：${relative(root, file).replaceAll('\\', '/')}`)
    }
  }

  let initialScriptBytes = 0
  for (const reference of initialScripts) {
    initialScriptBytes += (await stat(resolveInside(root, reference))).size
  }
  if (initialScriptBytes > initialScriptBudget) {
    throw new Error(
      `首屏 JavaScript 为 ${Math.ceil(initialScriptBytes / 1024)} KiB，超过 ${Math.floor(initialScriptBudget / 1024)} KiB 预算`,
    )
  }

  return {
    checkedReferences: references.size,
    initialScriptBytes,
  }
}

const executedPath = process.argv[1] ? resolve(process.argv[1]) : null
if (executedPath === fileURLToPath(import.meta.url)) {
  const outputDirectory = process.argv[2] || 'apps/web/.output/public'
  try {
    const result = await verifyStaticOutput(outputDirectory)
    console.log(
      `静态产物校验通过：${result.checkedReferences} 个资源引用，首屏 JavaScript ${Math.ceil(result.initialScriptBytes / 1024)} KiB`,
    )
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error))
    process.exitCode = 1
  }
}
