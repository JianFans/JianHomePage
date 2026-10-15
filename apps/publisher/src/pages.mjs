import { isAbsolute } from 'node:path'

const statuses = new Map([
  ['Pending', 'building'], ['Process', 'building'], ['Success', 'succeeded'],
  ['Failed', 'failed'], ['Timeout', 'failed'], ['Cancelled', 'failed'], ['Invalid', 'failed'],
])

/** Only unambiguous Production identities may be attached to a publish task. */
function validateDeployment(deployment, projectId, deploymentId) {
  if (typeof deployment?.deploymentId !== 'string' || !deployment.deploymentId.trim()
    || deployment.projectId !== projectId || deployment.env !== 'Production'
    || (deploymentId !== undefined && deployment.deploymentId !== deploymentId)) {
    throw new Error('Invalid Pages deployment')
  }
}

/** Provider URLs must never expose signed parameters or embedded credentials. */
function safePreviewUrl(value) {
  try {
    if (typeof value !== 'string') return undefined
    const url = new URL(value)
    if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash) return undefined
    return url.href
  } catch { return undefined }
}

/**
 * Adapt an official Makers instance without adding retries to deployment writes.
 * @param {{deployments: {deploy: Function, get: Function}}} client
 * @returns {{deploy: Function, get: Function}}
 */
export function createPagesProvider(client) {
  return {
    /** Submit once and retain only the identity needed for durable reconciliation. */
    async deploy(directory, projectId) {
      if (typeof directory !== 'string' || !isAbsolute(directory) || typeof projectId !== 'string' || !projectId.trim()) {
        throw new Error('Invalid Pages deployment')
      }
      let deployment
      try {
        deployment = await client.deployments.deploy({ projectId, artifact: { directory }, env: 'Production', wait: false })
      } catch { throw new Error('Pages deployment request failed') }
      validateDeployment(deployment, projectId)
      return { deploymentId: deployment.deploymentId, projectId, env: 'Production' }
    },
    /** Read one known deployment; unknown provider states must preserve the active task. */
    async get(projectId, deploymentId) {
      if (typeof projectId !== 'string' || !projectId.trim() || typeof deploymentId !== 'string' || !deploymentId.trim()) {
        throw new Error('Invalid Pages deployment')
      }
      let deployment
      try { deployment = await client.deployments.get({ projectId, deploymentId }) }
      catch { throw new Error('Pages deployment query failed') }
      validateDeployment(deployment, projectId, deploymentId)
      const status = statuses.get(deployment.status)
      if (!status) throw new Error('Unknown Pages deployment status')
      return {
        deploymentId, projectId, env: 'Production', status,
        previewUrl: safePreviewUrl(deployment.previewUrl),
        error: status === 'failed' ? 'Pages deployment failed' : undefined,
      }
    },
  }
}
