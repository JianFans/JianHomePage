import { GetObjectCommand } from '@aws-sdk/client-s3'

/** Stream a configured private bucket; cancellation closes the HTTP body as well as the initial request. */
export function createSnapshotReader(client, bucket) {
  return async function* readSnapshot(key, { signal } = {}) {
    let body
    const abort = () => body?.destroy(new Error('Snapshot read aborted'))
    try {
      signal?.throwIfAborted()
      const response = await client.send(new GetObjectCommand({ Bucket: bucket, Key: key }), { abortSignal: signal })
      body = response.Body
      if (!body || typeof body.destroy !== 'function') throw new Error('Missing response body')
      signal?.addEventListener('abort', abort, { once: true })
      if (signal?.aborted) abort()
      for await (const chunk of body) yield chunk
    } catch { throw new Error('Snapshot read failed') }
    finally {
      signal?.removeEventListener('abort', abort)
      body?.destroy()
    }
  }
}
