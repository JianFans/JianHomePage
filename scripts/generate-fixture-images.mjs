import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import sharp from 'sharp'

const palette = {
  background: '#0d0f11',
  ink: '#111517',
  surface: '#192022',
  elevated: '#263034',
  mist: '#8b9696',
  pale: '#d9ddd8',
  blue: '#72888d',
  copper: '#9a7d6d',
}

function mulberry32(seed) {
  return () => {
    let value = seed += 0x6D2B79F5
    value = Math.imul(value ^ value >>> 15, value | 1)
    value ^= value + Math.imul(value ^ value >>> 7, value | 61)
    return ((value ^ value >>> 14) >>> 0) / 4294967296
  }
}

function frame(width, height, content) {
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${width}" height="${height}" viewBox="0 0 ${width} ${height}">
    <defs>
      <filter id="grain" x="0" y="0" width="100%" height="100%">
        <feTurbulence type="fractalNoise" baseFrequency="0.72" numOctaves="3" seed="17" />
        <feColorMatrix values="0.18 0 0 0 0  0 0.18 0 0 0  0 0 0.18 0 0  0 0 0 0.22 0" />
      </filter>
    </defs>
    <rect width="${width}" height="${height}" fill="${palette.background}" />
    ${content}
    <rect width="${width}" height="${height}" filter="url(#grain)" opacity="0.55" />
  </svg>`
}

function studio(width, height, seed) {
  const random = mulberry32(seed)
  const panels = Array.from({ length: 14 }, (_, index) => {
    const panelWidth = width * 0.045
    const x = width * 0.08 + index * panelWidth * 1.18
    const panelHeight = height * (0.34 + random() * 0.13)
    return `<rect x="${x}" y="${height * 0.12}" width="${panelWidth}" height="${panelHeight}" fill="${index % 3 === 0 ? palette.blue : palette.surface}" opacity="${0.42 + random() * 0.28}" />`
  }).join('')
  const controls = Array.from({ length: 18 }, (_, index) => {
    const x = width * 0.16 + (index % 9) * width * 0.078
    const y = height * 0.7 + Math.floor(index / 9) * height * 0.09
    const warm = index % 5 === 0
    return `<rect x="${x}" y="${y}" width="${width * 0.045}" height="${height * 0.012}" fill="${warm ? palette.copper : palette.mist}" opacity="${warm ? 0.85 : 0.42}" />
      <circle cx="${x + width * 0.022}" cy="${y + height * 0.042}" r="${height * 0.009}" fill="${palette.pale}" opacity="0.34" />`
  }).join('')

  return frame(width, height, `
    <rect x="${width * 0.055}" y="${height * 0.075}" width="${width * 0.89}" height="${height * 0.5}" fill="${palette.ink}" />
    ${panels}
    <rect x="${width * 0.69}" y="${height * 0.17}" width="${width * 0.19}" height="${height * 0.25}" fill="${palette.elevated}" opacity="0.82" />
    <rect x="${width * 0.71}" y="${height * 0.19}" width="${width * 0.15}" height="${height * 0.21}" fill="${palette.background}" />
    <path d="M ${width * 0.06} ${height * 0.95} L ${width * 0.17} ${height * 0.58} L ${width * 0.83} ${height * 0.58} L ${width * 0.96} ${height * 0.95} Z" fill="${palette.surface}" />
    <path d="M ${width * 0.14} ${height * 0.91} L ${width * 0.22} ${height * 0.63} L ${width * 0.78} ${height * 0.63} L ${width * 0.87} ${height * 0.91} Z" fill="${palette.elevated}" opacity="0.72" />
    ${controls}
  `)
}

function stage(width, height, seed) {
  const random = mulberry32(seed)
  const folds = Array.from({ length: 16 }, (_, index) => {
    const foldWidth = width * 0.025
    const side = index < 8 ? index : 24 + index
    const x = side * foldWidth
    return `<path d="M ${x} 0 L ${x + foldWidth * 1.25} 0 L ${x + foldWidth * 0.7} ${height * 0.72} L ${x - foldWidth * 0.1} ${height * 0.84} Z" fill="${index % 2 ? palette.ink : palette.surface}" opacity="${0.72 + random() * 0.18}" />`
  }).join('')
  const floorLines = Array.from({ length: 7 }, (_, index) => {
    const y = height * (0.74 + index * 0.04)
    return `<rect x="0" y="${y}" width="${width}" height="${Math.max(2, height * 0.004)}" fill="${palette.mist}" opacity="${0.18 - index * 0.015}" />`
  }).join('')

  return frame(width, height, `
    <rect x="${width * 0.2}" y="${height * 0.1}" width="${width * 0.6}" height="${height * 0.58}" fill="${palette.ink}" />
    <rect x="${width * 0.23}" y="${height * 0.16}" width="${width * 0.54}" height="${height * 0.45}" fill="${palette.surface}" opacity="0.7" />
    <path d="M ${width * 0.28} ${height * 0.04} L ${width * 0.44} ${height * 0.67} L ${width * 0.54} ${height * 0.67} L ${width * 0.39} ${height * 0.04} Z" fill="${palette.blue}" opacity="0.22" />
    <path d="M ${width * 0.7} ${height * 0.02} L ${width * 0.56} ${height * 0.67} L ${width * 0.65} ${height * 0.67} L ${width * 0.8} ${height * 0.02} Z" fill="${palette.copper}" opacity="0.2" />
    ${folds}
    <rect x="0" y="${height * 0.7}" width="${width}" height="${height * 0.3}" fill="${palette.ink}" />
    ${floorLines}
    <rect x="${width * 0.495}" y="${height * 0.39}" width="${width * 0.01}" height="${height * 0.34}" fill="${palette.mist}" opacity="0.58" />
    <rect x="${width * 0.485}" y="${height * 0.37}" width="${width * 0.055}" height="${height * 0.025}" rx="${height * 0.01}" fill="${palette.copper}" />
  `)
}

function cover(width, height, seed, variant) {
  const random = mulberry32(seed)
  const grid = Array.from({ length: 22 }, (_, index) => {
    const x = random() * width
    const y = random() * height
    const size = width * (0.008 + random() * 0.025)
    const color = index % 5 === 0 ? palette.copper : index % 3 === 0 ? palette.blue : palette.mist
    return `<rect x="${x}" y="${y}" width="${size}" height="${size * (0.3 + random())}" fill="${color}" opacity="${0.12 + random() * 0.28}" />`
  }).join('')
  const motifs = [
    `<circle cx="${width * 0.58}" cy="${height * 0.48}" r="${width * 0.29}" fill="${palette.ink}" stroke="${palette.mist}" stroke-width="${width * 0.012}" opacity="0.88" />
     <circle cx="${width * 0.58}" cy="${height * 0.48}" r="${width * 0.2}" fill="none" stroke="${palette.elevated}" stroke-width="${width * 0.008}" />
     <rect x="${width * 0.13}" y="${height * 0.16}" width="${width * 0.17}" height="${height * 0.68}" fill="${palette.copper}" opacity="0.82" />`,
    `<path d="M ${width * 0.12} ${height * 0.22} L ${width * 0.88} ${height * 0.12} L ${width * 0.7} ${height * 0.78} L ${width * 0.2} ${height * 0.9} Z" fill="${palette.surface}" />
     <path d="M ${width * 0.2} ${height * 0.32} L ${width * 0.78} ${height * 0.24} L ${width * 0.62} ${height * 0.7} L ${width * 0.3} ${height * 0.76} Z" fill="${palette.blue}" opacity="0.65" />`,
    `<rect x="${width * 0.18}" y="${height * 0.18}" width="${width * 0.64}" height="${height * 0.64}" fill="${palette.surface}" />
     ${Array.from({ length: 11 }, (_, index) => `<rect x="${width * 0.24}" y="${height * (0.27 + index * 0.043)}" width="${width * (0.3 + 0.18 * Math.sin(index * 1.7) ** 2)}" height="${height * 0.01}" fill="${index === 7 ? palette.copper : palette.mist}" opacity="${index === 7 ? 0.84 : 0.34}" />`).join('')}`,
    `<path d="M 0 ${height * 0.72} L ${width * 0.28} ${height * 0.5} L ${width * 0.48} ${height * 0.64} L ${width * 0.74} ${height * 0.3} L ${width} ${height * 0.46} L ${width} ${height} L 0 ${height} Z" fill="${palette.elevated}" />
     <rect x="${width * 0.16}" y="${height * 0.13}" width="${width * 0.56}" height="${height * 0.12}" fill="${palette.copper}" opacity="0.68" />`,
    `<rect x="${width * 0.1}" y="${height * 0.12}" width="${width * 0.8}" height="${height * 0.76}" fill="${palette.ink}" />
     ${Array.from({ length: 7 }, (_, index) => `<rect x="${width * (0.17 + index * 0.1)}" y="${height * 0.2}" width="${width * 0.055}" height="${height * (0.34 + (index % 3) * 0.09)}" fill="${index === 4 ? palette.copper : palette.blue}" opacity="${0.38 + index * 0.045}" />`).join('')}`,
  ]

  return frame(width, height, `${motifs[variant % motifs.length]}${grid}`)
}

function videoPoster(width, height, seed, variant) {
  const random = mulberry32(seed)
  const blocks = Array.from({ length: 12 }, (_, index) => {
    const blockWidth = width * (0.035 + random() * 0.1)
    const blockHeight = height * (0.08 + random() * 0.25)
    return `<rect x="${random() * (width - blockWidth)}" y="${random() * (height - blockHeight)}" width="${blockWidth}" height="${blockHeight}" fill="${index % 4 === variant ? palette.copper : palette.blue}" opacity="${0.08 + random() * 0.2}" />`
  }).join('')

  return frame(width, height, `
    <rect x="${width * 0.05}" y="${height * 0.08}" width="${width * 0.9}" height="${height * 0.84}" fill="${palette.ink}" />
    <rect x="${width * (0.14 + variant * 0.05)}" y="${height * 0.16}" width="${width * (0.55 - variant * 0.04)}" height="${height * 0.56}" fill="${palette.surface}" />
    <path d="M ${width * (0.26 + variant * 0.05)} ${height * 0.15} L ${width * 0.48} ${height * 0.78} L ${width * 0.68} ${height * 0.78} L ${width * (0.48 + variant * 0.04)} ${height * 0.15} Z" fill="${variant === 1 ? palette.copper : palette.blue}" opacity="0.22" />
    ${blocks}
    <rect x="0" y="${height * 0.84}" width="${width}" height="${height * 0.16}" fill="${palette.background}" />
  `)
}

function moment(width, height, seed, variant) {
  const random = mulberry32(seed)
  const strips = Array.from({ length: 13 }, (_, index) => {
    const x = width * (0.08 + index * 0.067)
    const top = height * (0.07 + random() * 0.18)
    const stripHeight = height * (0.4 + random() * 0.42)
    return `<rect x="${x}" y="${top}" width="${width * 0.035}" height="${stripHeight}" fill="${index % 4 === variant ? palette.copper : palette.elevated}" opacity="${0.22 + random() * 0.32}" />`
  }).join('')

  return frame(width, height, `
    <path d="M ${width * 0.08} ${height * 0.12} L ${width * 0.84} ${height * 0.06} L ${width * 0.93} ${height * 0.82} L ${width * 0.18} ${height * 0.94} Z" fill="${palette.surface}" />
    ${strips}
    <rect x="${width * (0.16 + variant * 0.08)}" y="${height * 0.7}" width="${width * 0.54}" height="${height * 0.12}" fill="${palette.blue}" opacity="0.4" />
    <rect x="${width * 0.62}" y="${height * 0.18}" width="${width * 0.17}" height="${height * 0.28}" fill="${palette.ink}" stroke="${palette.mist}" stroke-width="${width * 0.008}" />
  `)
}

const definitions = [
  ['hero-studio.webp', 1920, 1200, studio(1920, 1200, 101)],
  ['hero-stage.webp', 1920, 1200, stage(1920, 1200, 202)],
  ...Array.from({ length: 5 }, (_, index) => [
    `cover-0${index + 1}.webp`, 1200, 1200, cover(1200, 1200, 300 + index, index),
  ]),
  ...Array.from({ length: 3 }, (_, index) => [
    `video-0${index + 1}.webp`, 1600, 900, videoPoster(1600, 900, 400 + index, index),
  ]),
  ['moment-01.webp', 1200, 1500, moment(1200, 1500, 501, 0)],
  ['moment-02.webp', 1200, 1200, moment(1200, 1200, 502, 1)],
  ['moment-03.webp', 1200, 1600, moment(1200, 1600, 503, 2)],
]

const mediaDirectory = new URL('../apps/web/public/media/', import.meta.url)
await mkdir(mediaDirectory, { recursive: true })

const generated = new Map()
for (const [filename, width, height, svg] of definitions) {
  const buffer = await sharp(Buffer.from(svg))
    .webp({ quality: 86, effort: 6 })
    .toBuffer()
  await writeFile(new URL(filename, mediaDirectory), buffer)
  generated.set(`/media/${filename}`, {
    width,
    height,
    byteSize: buffer.length,
    checksum: `sha256:${createHash('sha256').update(buffer).digest('hex')}`,
  })
}

const fixtureUrl = new URL('../content/fixtures/homepage.json', import.meta.url)
const fixture = JSON.parse(await readFile(fixtureUrl, 'utf8'))
for (const asset of fixture.assets) {
  const metadata = generated.get(asset.src)
  if (!metadata) {
    continue
  }

  asset.width = metadata.width
  asset.height = metadata.height
  asset.byteSize = metadata.byteSize
  asset.checksum = metadata.checksum
  asset.rights.source = {
    'zh-CN': '本地程序化生成的测试素材',
    en: 'Locally generated procedural fixture asset',
  }
}

await writeFile(fixtureUrl, `${JSON.stringify(fixture, null, 2)}\n`)

console.log(`Generated ${definitions.length} WebP fixtures in ${fileURLToPath(mediaDirectory)}`)
