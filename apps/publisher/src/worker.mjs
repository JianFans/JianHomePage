import { isDeepStrictEqual } from 'node:util'

/** An operational conflict is safe to expose without provider or database internals. */
export class ConflictError extends Error {
  /** Keep conflicts distinguishable across the HTTP boundary without inspecting messages. */
  constructor(message) { super(message); this.name = 'ConflictError' }
}

/** Refuse binding another project's deployment, even if a provider adapter is replaced. */
function verifyDeployment(deployment, target, id) {
  if (!deployment?.deploymentId || (id && deployment.deploymentId !== id)
    || deployment.projectId !== target.projectId || deployment.env !== 'Production') {
    throw new ConflictError('Deployment identity or environment mismatch')
  }
}

/** Expose the existing Go BuildTrigger response while retaining diagnostic phase information. */
export function buildResponse(job) {
  const status = job.phase === 'queued' ? 'pending' : ['succeeded', 'failed'].includes(job.phase) ? job.phase : 'building'
  return { id: job.id, status, phase: job.phase, deploymentId: job.deploymentId || '', previewUrl: job.previewUrl || '', error: job.error || '' }
}

/** Advance a durable task only while the caller holds the global publishing database lock. */
export function createWorker({ store, target, pages, build }) {
  return {
    /** Build safely repeatable phases; never resubmit an ambiguous deployment creation. */
    async tick() {
      let job = await store.active()
      if (!job) return
      if (!isDeepStrictEqual(job.target, target)) {
        await store.update(job.id, job.phase, { error: 'Frozen publisher configuration differs; restore the original configuration' })
        return
      }
      if (job.phase === 'submitting') {
        if (!job.error) await store.update(job.id, job.phase, { error: 'Deployment outcome unknown; reconciliation required' })
        return
      }
      if (['queued', 'building'].includes(job.phase)) {
        job = await store.update(job.id, job.phase, { phase: 'building', error: '' })
        let directory
        try {
          directory = await build({ id: job.id, ...job.request })
        } catch {
          await store.update(job.id, 'building', { phase: 'failed', error: 'Snapshot validation or static build failed' })
          return
        }
        // This write must succeed before invoking a non-idempotent cloud operation.
        await store.update(job.id, 'building', { phase: 'submitting' })
        let deployment
        try {
          deployment = await pages.deploy(directory, job.target.projectId)
          verifyDeployment(deployment, job.target)
        } catch {
          await store.update(job.id, 'submitting', { error: 'Deployment outcome unknown; reconciliation required' })
          return
        }
        // A failed DB write leaves submitting intact for explicit recovery, never failed/retry.
        await store.update(job.id, 'submitting', { phase: 'deployed', deploymentId: deployment.deploymentId })
        return
      }
      if (job.phase === 'deployed') {
        let deployment
        try {
          deployment = await pages.get(job.target.projectId, job.deploymentId)
          verifyDeployment(deployment, job.target, job.deploymentId)
          if (!['building', 'succeeded', 'failed'].includes(deployment.status)) throw new Error('Unknown deployment status')
        } catch {
          await store.update(job.id, 'deployed', { error: 'Deployment status unavailable; automatic reconciliation will continue' })
          return
        }
        const phase = deployment.status === 'building' ? 'deployed' : deployment.status
        await store.update(job.id, 'deployed', {
          phase, error: phase === 'failed' ? 'Pages deployment failed' : '',
          previewUrl: phase === 'succeeded' ? job.target.publicUrl : '',
        })
      }
    },

    /** Record an operator's verified resolution without retrying a potentially completed upload. */
    async reconcile(id, input) {
      const job = await store.get(id)
      if (!job || job.phase !== 'submitting' || !isDeepStrictEqual(job.target, target)) {
        throw new ConflictError('Only an uncertain deployment with matching configuration can be reconciled')
      }
      if (!input || typeof input.note !== 'string' || !input.note.trim() || input.note.length > 500 || /\p{Cc}/u.test(input.note)) {
        throw new ConflictError('A concise verification note is required')
      }
      let fields
      if (typeof input.deploymentId === 'string' && /^[a-zA-Z0-9_-]{1,128}$/.test(input.deploymentId) && input.confirmedAbsent === undefined) {
        const deployment = await pages.get(job.target.projectId, input.deploymentId)
        verifyDeployment(deployment, job.target, input.deploymentId)
        fields = { phase: 'deployed', deploymentId: input.deploymentId, error: '' }
      } else if (input.confirmedAbsent === true && input.deploymentId === undefined) {
        fields = { phase: 'failed', error: 'Operator confirmed that no deployment was created' }
      } else {
        throw new ConflictError('Provide a verified deployment ID or explicit confirmedAbsent')
      }
      const reconciliation = [...job.reconciliation, { ...input, note: input.note.trim(), at: new Date().toISOString() }]
      return store.update(id, 'submitting', { ...fields, reconciliation })
    },
  }
}
