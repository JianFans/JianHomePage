import { createHash } from 'node:crypto'
import { mkdir, readFile, writeFile } from 'node:fs/promises'

const sampleRate = 44_100
const durationSeconds = 3
const sampleCount = sampleRate * durationSeconds
const bytesPerSample = 2
const dataSize = sampleCount * bytesPerSample
const output = Buffer.alloc(44 + dataSize)

output.write('RIFF', 0)
output.writeUInt32LE(36 + dataSize, 4)
output.write('WAVE', 8)
output.write('fmt ', 12)
output.writeUInt32LE(16, 16)
output.writeUInt16LE(1, 20)
output.writeUInt16LE(1, 22)
output.writeUInt32LE(sampleRate, 24)
output.writeUInt32LE(sampleRate * bytesPerSample, 28)
output.writeUInt16LE(bytesPerSample, 32)
output.writeUInt16LE(16, 34)
output.write('data', 36)
output.writeUInt32LE(dataSize, 40)

for (let index = 0; index < sampleCount; index += 1) {
  const time = index / sampleRate
  const edgeSeconds = Math.min(time, durationSeconds - time)
  const envelope = Math.min(1, edgeSeconds / 0.08)
  const fundamental = Math.sin(2 * Math.PI * 220 * time)
  const harmonic = Math.sin(2 * Math.PI * 330 * time) * 0.25
  const sample = Math.round((fundamental + harmonic) * envelope * 1_800)
  output.writeInt16LE(sample, 44 + index * bytesPerSample)
}

const outputUrl = new URL('../apps/web/public/media/preview-sample.wav', import.meta.url)
await mkdir(new URL('../apps/web/public/media/', import.meta.url), { recursive: true })
await writeFile(outputUrl, output)

const fixtureUrl = new URL('../content/fixtures/homepage.json', import.meta.url)
const fixture = JSON.parse(await readFile(fixtureUrl, 'utf8'))
const asset = fixture.assets.find(({ src }) => src === '/media/preview-sample.wav')

if (!asset) {
  throw new Error('Audio fixture asset is missing from content snapshot')
}

asset.byteSize = output.length
asset.durationSeconds = durationSeconds
asset.checksum = `sha256:${createHash('sha256').update(output).digest('hex')}`

await writeFile(fixtureUrl, `${JSON.stringify(fixture, null, 2)}\n`)
