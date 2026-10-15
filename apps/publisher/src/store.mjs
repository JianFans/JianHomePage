import { createHash } from 'node:crypto'
import { isDeepStrictEqual } from 'node:util'
import { ConflictError } from './worker.mjs'

const LOCK_ID = 865734220
const activePhases = ['queued', 'building', 'submitting', 'deployed']
const fields = { phase: 'phase', deploymentId: 'deployment_id', previewUrl: 'preview_url', error: 'error', reconciliation: 'reconciliation' }

/** Convert database naming only; provider configuration remains an immutable JSON value. */
function record(row) {
  if (!row) return null
  return {
    id: row.id, request: row.request, target: row.target, phase: row.phase,
    deploymentId: row.deployment_id, previewUrl: row.preview_url, error: row.error,
    reconciliation: row.reconciliation, createdAt: row.created_at,
  }
}

/** Create a PostgreSQL-backed repository; locked worker operations use their own pinned connection. */
export function createStore(database, { onLockLost = () => {} } = {}) {
  return {
    /** Check the required migration at startup and for readiness, rather than falling back to memory. */
    async health() {
      await database.query('SELECT id, phase, target, reconciliation FROM publisher_builds LIMIT 0')
    },

    /** Persist one logical build per key, preserving its original target across retries. */
    async enqueue(key, request, target) {
      const id = `build_${createHash('sha256').update(key).digest('hex')}`
      let row
      try {
        const result = await database.query(`INSERT INTO publisher_builds (id, idempotency_key, request, target, phase)
          VALUES ($1, $2, $3::jsonb, $4::jsonb, 'queued')
          ON CONFLICT (idempotency_key) DO NOTHING RETURNING *`, [id, key, JSON.stringify(request), JSON.stringify(target)])
        row = result.rows[0]
      } catch (error) {
        if (error.code !== '23505') throw error
        // The partial active-slot index can win the race before the key index.
      }
      if (!row) row = (await database.query('SELECT * FROM publisher_builds WHERE idempotency_key = $1', [key])).rows[0]
      if (!row || !isDeepStrictEqual(row.request, request)) throw new ConflictError('Idempotency key conflict or another active build')
      return record(row)
    },

    /** Read a stable task ID without exposing the shared gateway credential. */
    async get(id) {
      return record((await database.query('SELECT * FROM publisher_builds WHERE id = $1', [id])).rows[0])
    },

    /** The partial unique index guarantees this query has at most one result. */
    async active() {
      return record((await database.query('SELECT * FROM publisher_builds WHERE phase = ANY($1::text[]) LIMIT 1', [activePhases])).rows[0])
    },

    /** Apply a phase-checked transition; SQL identifiers come exclusively from the fixed whitelist. */
    async update(id, expectedPhase, changes) {
      const entries = Object.entries(changes)
      if (!entries.length || entries.some(([key]) => !Object.hasOwn(fields, key))) throw new Error('Invalid publisher update')
      const assignments = entries.map(([key], index) => `${fields[key]} = $${index + 3}${key === 'reconciliation' ? '::jsonb' : ''}`)
      const values = entries.map(([key, value]) => key === 'reconciliation' ? JSON.stringify(value) : value)
      const result = await database.query(`UPDATE publisher_builds SET ${assignments.join(', ')}, updated_at = now() WHERE id = $1 AND phase = $2 RETURNING *`, [id, expectedPhase, ...values])
      if (result.rowCount !== 1) throw new ConflictError('Publisher task changed')
      return record(result.rows[0])
    },

    /** Serialize workers and manual recovery; a lost session invalidates all further writes. */
    async withLock(operation) {
      const client = await database.connect()
      let locked = false
      let lost = false
      const connectionLost = () => { lost = true; onLockLost() }
      client.on('error', connectionLost)
      try {
        locked = (await client.query('SELECT pg_try_advisory_lock($1) AS locked', [LOCK_ID])).rows[0].locked
        if (!locked) return null
        const guarded = {
          /** Refuse all phase changes after the session holding the publishing lock is lost. */
          async query(...args) {
            if (lost) throw new Error('Publisher lock lost')
            return client.query(...args)
          },
        }
        return await operation(createStore(guarded))
      } finally {
        try {
          if (locked && !lost) await client.query('SELECT pg_advisory_unlock($1)', [LOCK_ID])
        } catch {
          lost = true
          onLockLost()
        } finally {
          client.removeListener('error', connectionLost)
          client.release(lost)
        }
      }
    },
  }
}
