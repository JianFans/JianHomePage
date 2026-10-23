export type SnapshotChangeKind = 'added' | 'removed' | 'changed' | 'reordered'
export interface SnapshotChange {
  kind: SnapshotChangeKind
  path: string
  beforePath?: string
  afterPath?: string
  before?: unknown
  after?: unknown
}
export interface SnapshotDiffGroup { group: string; changes: SnapshotChange[] }

const groupOrder = ['site', 'artist', 'heroSlides', 'releases', 'tracks', 'videos', 'events', 'moments', 'assets', 'homepage']
/** Escape a single JSON Pointer segment, including stable record identities. */
const escapePath = (key: string) => key.replace(/~/g, '~0').replace(/\//g, '~1')
/** Restrict field traversal to non-null objects; arrays have separate identity rules. */
const isObject = (value: unknown): value is Record<string, unknown> => !!value && typeof value === 'object' && !Array.isArray(value)

/** Compare values without treating JSON object property order as a content change. */
export function equalSnapshotValue(before: unknown, after: unknown): boolean {
  if (before === after) return true
  if (Array.isArray(before) && Array.isArray(after)) return before.length === after.length && before.every((value, index) => equalSnapshotValue(value, after[index]))
  if (!isObject(before) || !isObject(after)) return false
  const keys = Object.keys(before)
  return keys.length === Object.keys(after).length && keys.every(key => Object.hasOwn(after, key) && equalSnapshotValue(before[key], after[key]))
}

/** Only unique string IDs can safely align records across differently ordered arrays. */
function recordArray(value: unknown[]): value is (Record<string, unknown> & { id: string })[] {
  return value.every(item => isObject(item) && typeof item.id === 'string') && new Set(value.map(item => (item as { id: string }).id)).size === value.length
}

/** Identity paths use /@ID for records; beforePath and afterPath are exact JSON Pointers. */
export function diffSnapshots(before: Record<string, unknown>, after: Record<string, unknown>): SnapshotDiffGroup[] {
  const groups: SnapshotDiffGroup[] = []
  const keys = new Set([...Object.keys(before), ...Object.keys(after)])
  const ordered = [...groupOrder.filter(key => keys.has(key)), ...[...keys].filter(key => !groupOrder.includes(key)).sort()]
  for (const group of ordered) {
    const changes: SnapshotChange[] = []
    /** Keep stable display identities separate from the exact pointers in each snapshot. */
    function walk(left: unknown, right: unknown, path: string, beforePath?: string, afterPath?: string) {
      if (beforePath !== undefined && afterPath !== undefined && equalSnapshotValue(left, right)) return
      /** Omit missing-side values instead of confusing absent fields with null. */
      const emit = (kind: SnapshotChangeKind, oldValue = left, newValue = right) => changes.push({
        kind, path, ...(beforePath !== undefined ? { beforePath, before: oldValue } : {}), ...(afterPath !== undefined ? { afterPath, after: newValue } : {}),
      })
      if (beforePath === undefined) { emit('added'); return }
      if (afterPath === undefined) { emit('removed'); return }
      if (Array.isArray(left) && Array.isArray(right)) {
        if (recordArray(left) && recordArray(right)) {
          const oldRecords = new Map(left.map((item, index) => [item.id, { item, index }]))
          const newRecords = new Map(right.map((item, index) => [item.id, { item, index }]))
          const oldOrder = left.filter(item => newRecords.has(item.id)).map(item => item.id)
          const newOrder = right.filter(item => oldRecords.has(item.id)).map(item => item.id)
          if (!equalSnapshotValue(oldOrder, newOrder)) emit('reordered', left.map(item => item.id), right.map(item => item.id))
          for (const id of new Set([...oldRecords.keys(), ...newRecords.keys()])) {
            const oldRecord = oldRecords.get(id)
            const newRecord = newRecords.get(id)
            walk(oldRecord?.item, newRecord?.item, `${path}/@${escapePath(id)}`, oldRecord ? `${beforePath}/${oldRecord.index}` : undefined, newRecord ? `${afterPath}/${newRecord.index}` : undefined)
          }
        } else {
          // Primitive reference arrays may contain duplicate IDs; compare multiplicities as well as order.
          const counts = new Map<unknown, number>()
          for (const item of left) counts.set(item, (counts.get(item) || 0) + 1)
          for (const item of right) counts.set(item, (counts.get(item) || 0) - 1)
          emit(left.every(item => item === null || typeof item !== 'object') && left.length === right.length && [...counts.values()].every(count => count === 0) ? 'reordered' : 'changed')
        }
        return
      }
      if (isObject(left) && isObject(right)) {
        for (const key of [...new Set([...Object.keys(left), ...Object.keys(right)])].sort()) {
          const segment = escapePath(key)
          walk(left[key], right[key], `${path}/${segment}`, Object.hasOwn(left, key) ? `${beforePath}/${segment}` : undefined, Object.hasOwn(right, key) ? `${afterPath}/${segment}` : undefined)
        }
        return
      }
      emit('changed')
    }
    const path = `/${escapePath(group)}`
    walk(before[group], after[group], path, Object.hasOwn(before, group) ? path : undefined, Object.hasOwn(after, group) ? path : undefined)
    if (changes.length) groups.push({ group, changes })
  }
  return groups
}

/** Bound serialization work and DOM text even when a whole record or collection was added. */
export function formatDiffValue(value: unknown): string {
  if (value === undefined) return '—'
  let budget = 12000
  let nodes = 80
  /** Consume the shared text budget before serializing another field or item. */
  const clip = (text: string) => {
    const remaining = Math.max(0, budget)
    const result = text.slice(0, remaining)
    budget -= result.length
    return result + (text.length > remaining ? '…' : '')
  }
  /** Stop traversal at the node, depth or text budget, including expanded values. */
  function format(item: unknown, depth: number): string {
    if (--nodes < 0 || budget <= 0 || depth > 8) return '…'
    if (typeof item === 'string') return clip(item)
    if (Array.isArray(item)) {
      const values: string[] = []
      for (const child of item) {
        if (nodes <= 0 || budget <= 0) { values.push('…'); break }
        values.push(format(child, depth + 1))
      }
      return `[${values.join(', ')}]`
    }
    if (isObject(item)) {
      const values: string[] = []
      for (const key of Object.keys(item)) {
        if (nodes <= 0 || budget <= 0) { values.push('…'); break }
        values.push(`${clip(key)}: ${format(item[key], depth + 1)}`)
      }
      return `{${values.join(', ')}}`
    }
    return clip(String(item))
  }
  return format(value, 0)
}
