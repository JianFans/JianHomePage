import { mountSuspended } from '@nuxt/test-utils/runtime'
import { defineComponent, nextTick, reactive, ref, type Ref } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import { useAdminWorkspace } from '../../composables/useAdminWorkspace'
import type { AdminPublishJob, AdminVersion } from '../../utils/admin-api'
import type { AdminLocale } from '../../utils/admin-locale'

const fixture = fixtureData as unknown as Record<string, unknown>

/** 创建可由并发导入测试手动完成或拒绝的 Promise。 */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

/** 构造工作区 API 流程测试使用的 JSON 响应。 */
function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

/** 在 Nuxt 测试作用域中挂载组合式函数并返回可解包状态。 */
async function mountWorkspace(locale: Ref<AdminLocale> = ref('zh-CN'), session: Parameters<typeof useAdminWorkspace>[1] = {}) {
  const host = defineComponent({
    /** 向测试宿主暴露由 Vue 自动解包的工作区状态。 */
    setup() {
      return { workspace: reactive(useAdminWorkspace(locale, session)) }
    },
    template: '<div />',
  })
  const wrapper = await mountSuspended(host)
  return { wrapper, workspace: wrapper.vm.workspace }
}

/** 创建具有合法快照的版本测试对象。 */
function version(overrides: Partial<AdminVersion> = {}): AdminVersion {
  return {
    id: 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d',
    status: 'draft',
    revision: 1,
    snapshot: structuredClone(fixture),
    checksum: 'sha256:version',
    ...overrides,
  }
}

/** 创建可按场景覆盖状态的发布任务测试对象。 */
function publishJob(overrides: Partial<AdminPublishJob> = {}): AdminPublishJob {
  return {
    id: 'pub_b2549aa9-b67e-4031-93fc-d0edfa9de14a',
    versionId: 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d',
    status: 'building',
    snapshotKey: 'snapshots/ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d.json',
    snapshotChecksum: 'sha256:version',
    ...overrides,
  }
}

it('keeps the write lock if a concurrent summary request fails', async () => {
  const { wrapper, workspace } = await mountWorkspace()
  const list = deferred<Response>()
  const write = deferred<Response>()
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce(jsonResponse(version()))
    .mockReturnValueOnce(list.promise)
    .mockReturnValueOnce(write.promise))
  workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
  await workspace.loadVersion()
  const listing = workspace.listVersions()
  const saving = workspace.saveDraft()
  list.reject(new Error('Unavailable'))
  await listing
  expect(workspace.listError).toBe(true)
  expect(workspace.busy).toBe(true)
  write.resolve(jsonResponse(version({ revision: 2 })))
  await saving
  expect(workspace.version?.revision).toBe(2)
  wrapper.unmount()
})

it('ignores unauthorized responses from the previous session after recovery', async () => {
  const active = ref(true)
  const csrfToken = ref('old-csrf')
  const unauthorized = vi.fn()
  const old = deferred<Response>()
  vi.stubGlobal('fetch', vi.fn().mockReturnValueOnce(old.promise)
    .mockResolvedValueOnce(jsonResponse({ code: 'unauthorized', message: 'Unauthorized' }, 401)))
  const { wrapper, workspace } = await mountWorkspace(ref('en'), { active, csrfToken, onUnauthorized: unauthorized })
  try {
    const listing = workspace.listVersions()
    active.value = false
    csrfToken.value = ''
    await nextTick()
    csrfToken.value = 'new-csrf'
    active.value = true
    old.resolve(jsonResponse({ code: 'unauthorized', message: 'Unauthorized' }, 401))
    await listing
    expect(unauthorized).not.toHaveBeenCalled()
    await workspace.listVersions()
    expect(unauthorized).toHaveBeenCalledOnce()
  } finally { wrapper.unmount() }
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

it('重试失败任务使用新键及原任务版本，不替换当前选择', async () => {
  const { wrapper, workspace } = await mountWorkspace()
  const keys: string[] = []
  let attempt = 0
  vi.stubGlobal('fetch', vi.fn(async (input, init) => {
    if (String(input).endsWith('/publishes')) {
      keys.push(new Headers(init?.headers).get('Idempotency-Key')!)
      expect(JSON.parse(String(init?.body)).versionId).toBe('ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')
      return jsonResponse(publishJob({ id: `job_${++attempt}`, status: attempt === 1 ? 'failed' : 'building', operation: 'publish' }))
    }
    return jsonResponse(version({ status: 'in_review', reviewApproved: true }))
  }))
  workspace.version = version({ status: 'in_review', reviewApproved: true })
  await workspace.publish()
  workspace.version = version({ id: 'ver_other' })
  await workspace.retryPublish()
  expect(keys).toHaveLength(2)
  expect(keys[1]).not.toBe(keys[0])
  expect(workspace.publishJob?.id).toBe('job_2')
  expect(workspace.version?.id).toBe('ver_other')
  wrapper.unmount()
})

it('重试响应丢失后复用同一键，不能把一次回滚变成两次云端创建', async () => {
  const { wrapper, workspace } = await mountWorkspace()
  const old = publishJob({ status: 'failed', operation: 'rollback' })
  workspace.publishJob = old
  const keys: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (input, init) => {
    if (String(input).endsWith('/rollbacks')) {
      keys.push(new Headers(init?.headers).get('Idempotency-Key')!)
      if (keys.length === 1) throw new Error('Response lost')
      return jsonResponse(publishJob({ id: 'job_retry', operation: 'rollback' }))
    }
    if (String(input).endsWith('/latest')) return jsonResponse(old)
    return jsonResponse(version())
  }))
  await workspace.retryPublish()
  await workspace.retryPublish()
  expect(keys).toHaveLength(2)
  expect(keys[0]).toBeTruthy()
  expect(keys[1]).toBe(keys[0])
  expect(workspace.publishJob?.id).toBe('job_retry')
  wrapper.unmount()
})

describe('管理工作区', () => {
  it('保存后的新修订不会被迟到的旧摘要页覆盖', async () => {
    const { wrapper, workspace } = await mountWorkspace()
    const late = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(jsonResponse(version()))
      .mockReturnValueOnce(late.promise)
      .mockResolvedValueOnce(jsonResponse(version({ revision: 2 }))))
    workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
    await workspace.loadVersion()
    const listing = workspace.listVersions()
    workspace.editorText += '\n'
    await workspace.saveDraft()
    late.resolve(jsonResponse({ items: [version({ revision: 1 })] }))
    await listing
    expect(workspace.versions.find(item => item.id === 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')?.revision).toBe(2)
    expect(workspace.listing).toBe(false)
    wrapper.unmount()
  })
  it('历史只读不能保存，复制调用服务端并切入独立草稿', async () => {
    const { wrapper, workspace } = await mountWorkspace()
    const fetcher = vi.fn()
      .mockResolvedValueOnce(jsonResponse(version({ status: 'published' })))
      .mockResolvedValueOnce(jsonResponse(version({ id: 'ver_9cb0468e-f117-476b-be75-eed979e0c719', name: '新编排', sourceVersionId: 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d' })))
    vi.stubGlobal('fetch', fetcher)
    workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
    await workspace.loadVersion()
    expect(workspace.editable).toBe(false)
    await workspace.saveDraft()
    expect(fetcher).toHaveBeenCalledTimes(1)
    await workspace.cloneVersion('ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d', '新编排')
    expect(workspace.version?.id).toBe('ver_9cb0468e-f117-476b-be75-eed979e0c719')
    expect(workspace.editable).toBe(true)
    expect(workspace.dirty).toBe(false)
    expect(fetcher.mock.calls[1]?.[0]).toContain('/ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d/clone')
    expect(JSON.parse(fetcher.mock.calls[1]?.[1].body)).toEqual({ name: '新编排' })
    wrapper.unmount()
  })

  it('刷新版本列表时拒绝迟到页，按 ID 去重并保留服务端顺序', async () => {
    const { wrapper, workspace } = await mountWorkspace()
    const late = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn()
      .mockReturnValueOnce(late.promise)
      .mockResolvedValueOnce(jsonResponse({ items: [version({ id: 'ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526' })], nextCursor: 'page2' }))
      .mockResolvedValueOnce(jsonResponse({ items: [version({ id: 'ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526' }), version({ id: 'ver_b88f6af7-86cb-4fc4-b1dc-76722b17792e' })] })))
    const first = workspace.listVersions()
    await workspace.listVersions()
    late.resolve(jsonResponse({ items: [version({ id: 'ver_bf32989f-4df2-41c3-a592-4489f3041589' })] }))
    await first
    expect(workspace.versions.map(item => item.id)).toEqual(['ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526'])
    await workspace.listVersions(true)
    expect(workspace.versions.map(item => item.id)).toEqual(['ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526', 'ver_b88f6af7-86cb-4fc4-b1dc-76722b17792e'])
    expect(workspace.nextCursor).toBe('')
    wrapper.unmount()
  })

  it('销毁后保存响应不会改变本地内容或版本', async () => {
    const { wrapper, workspace } = await mountWorkspace()
    const late = deferred<Response>()
    vi.stubGlobal('fetch', vi.fn().mockReturnValueOnce(late.promise))
    workspace.editorText = JSON.stringify(fixture)
    const saving = workspace.saveDraft()
    wrapper.unmount()
    late.resolve(jsonResponse(version()))
    await saving
    expect(workspace.version).toBeNull()
  })
  it('暴露同一防抖文本的只读分析基线，输入变化不伪装成当前分析', async () => {
    const { wrapper, workspace } = await mountWorkspace()
    vi.useFakeTimers()
    expect(workspace.analyzedText).toBe('{}')
    workspace.editorText = JSON.stringify(fixture)
    expect(workspace.analyzedText).toBe('{}')
    await vi.advanceTimersByTimeAsync(250)
    expect(workspace.analyzedText).toBe(workspace.editorText)
    expect(workspace.editorAnalysis.snapshot).not.toBeNull()
    wrapper.unmount()
  })
  it('标记未保存文本并在保存冲突后保留本地内容和修订号', async () => {
    const { workspace } = await mountWorkspace()
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(jsonResponse(version()))
      .mockResolvedValueOnce(jsonResponse({ code: 'revision_conflict', message: 'Conflict', requestId: 'req_aea90e7b-6a5e-42cb-9962-2eba1e8cfc5f' }, 409)))
    workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
    await workspace.loadVersion()
    expect(workspace.dirty).toBe(false)
    const text = `${workspace.editorText}\n`
    workspace.editorText = text
    expect(workspace.dirty).toBe(true)
    expect(workspace.canSubmitReview).toBe(false)
    await workspace.saveDraft()
    expect(workspace.editorText).toBe(text)
    expect(workspace.version?.revision).toBe(1)
    expect(workspace.dirty).toBe(true)
    expect(workspace.workflow.requestId).toBe('req_aea90e7b-6a5e-42cb-9962-2eba1e8cfc5f')
  })

  it('保存期间拒绝并发载入和导入，迟到响应不会覆盖新文本', async () => {
    const { workspace } = await mountWorkspace()
    let resolveResponse!: (value: Response) => void
    const response = new Promise<Response>((resolve) => { resolveResponse = resolve })
    const fetcher = vi.fn().mockReturnValue(response)
    vi.stubGlobal('fetch', fetcher)
    workspace.editorText = JSON.stringify(fixture)
    const saving = workspace.saveDraft()
    workspace.versionId = 'ver_6e951bd5-bd9c-4f24-8450-4e9fe3c5ff49'
    await workspace.loadVersion()
    const read = vi.fn().mockResolvedValue('{}')
    await workspace.importSnapshot({ name: 'other.json', size: 2, text: read })
    await workspace.saveDraft()
    expect(fetcher).toHaveBeenCalledTimes(1)
    expect(read).not.toHaveBeenCalled()
    const newerText = `${workspace.editorText}\n`
    workspace.editorText = newerText
    resolveResponse(jsonResponse(version()))
    await saving
    expect(workspace.editorText).toBe(newerText)
    expect(workspace.version?.id).toBe('ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')
    expect(workspace.dirty).toBe(true)
  })

  it('拒绝空版本 ID 和无效快照', async () => {
    const { workspace } = await mountWorkspace()

    await workspace.loadVersion()
    expect(workspace.workflow).toMatchObject({ status: 'error', message: '请先填写版本 ID' })

    workspace.editorText = '[]'
    await workspace.saveDraft()
    expect(workspace.workflow).toMatchObject({ status: 'error', message: '快照必须是 JSON 对象' })
    expect(workspace.canSave).toBe(false)
  })

  it('分析、导入和导出完整快照', async () => {
    const { workspace } = await mountWorkspace()
    vi.useFakeTimers()
    const contents = JSON.stringify(fixture)

    workspace.editorText = contents
    await vi.advanceTimersByTimeAsync(1000)
    expect(workspace.editorAnalysis.issues).toHaveLength(0)
    expect(workspace.canSave).toBe(true)
    expect(workspace.exportSnapshot()).toMatchObject({
      filename: 'rel_14c9e5f2-718a-4271-b3e1-643f1a1d6ccc.json',
      mimeType: 'application/json',
    })

    workspace.editorText = '{}'
    await vi.advanceTimersByTimeAsync(1000)
    expect(workspace.canSave).toBe(false)
    expect(workspace.exportSnapshot()).toBeNull()

    await workspace.importSnapshot({
      name: 'draft.json',
      size: contents.length,
      text: async () => contents,
    })
    await vi.advanceTimersByTimeAsync(1000)
    expect(workspace.editorAnalysis.snapshot?.releaseId).toBe('rel_14c9e5f2-718a-4271-b3e1-643f1a1d6ccc')
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '已导入快照' })
  })

  it('防抖界面分析并用当前语言校验保存内容', async () => {
    const locale = ref<AdminLocale>('en')
    const { workspace } = await mountWorkspace(locale)
    vi.useFakeTimers()

    workspace.editorText = JSON.stringify(fixture)
    await nextTick()

    expect(workspace.editorAnalysis.snapshot).toBeNull()
    expect(workspace.canSave).toBe(false)

    await vi.advanceTimersByTimeAsync(1000)

    expect(workspace.editorAnalysis.snapshot?.releaseId).toBe('rel_14c9e5f2-718a-4271-b3e1-643f1a1d6ccc')
    expect(workspace.canSave).toBe(true)

    workspace.editorText = '[]'
    await nextTick()

    expect(workspace.editorAnalysis.snapshot?.releaseId).toBe('rel_14c9e5f2-718a-4271-b3e1-643f1a1d6ccc')
    expect(workspace.canSave).toBe(true)
    expect(workspace.exportSnapshot()).toBeNull()

    await workspace.saveDraft()

    expect(workspace.workflow).toMatchObject({ status: 'error', message: 'Snapshot must be a JSON object' })

    await vi.advanceTimersByTimeAsync(1000)

    expect(workspace.editorAnalysis.snapshot).toBeNull()
    expect(workspace.parsedEditor.error).toBe('Snapshot must be a JSON object')
    expect(workspace.canSave).toBe(false)

    locale.value = 'zh-CN'
    await nextTick()
    expect(workspace.parsedEditor.error).toBe('快照必须是 JSON 对象')
  })

  it('拒绝无法读取的导入文件', async () => {
    const { workspace } = await mountWorkspace()

    await workspace.importSnapshot({
      name: 'draft.txt',
      size: 2,
      text: async () => '{}',
    }, 'en')

    expect(workspace.workflow).toMatchObject({ status: 'error', message: 'Choose a JSON file' })
  })

  it('只提交最后一次异步导入的结果，并在导入期间关闭保存门禁', async () => {
    const { workspace } = await mountWorkspace()
    const first = deferred<string>()
    const second = deferred<string>()
    const firstContents = JSON.stringify({ ...fixture, releaseId: 'rel_9f13c342-c17e-4887-a9be-e6ae3e63ac16' })
    const secondContents = JSON.stringify({ ...fixture, releaseId: 'rel_2a788c8b-8417-4da8-a053-8687d3bf5a49' })

    const firstImport = workspace.importSnapshot({
      name: 'first.json',
      size: firstContents.length,
      text: () => first.promise,
    })
    const secondImport = workspace.importSnapshot({
      name: 'second.json',
      size: secondContents.length,
      text: () => second.promise,
    })

    expect(workspace.importing).toBe(true)
    expect(workspace.canSave).toBe(false)

    second.resolve(secondContents)
    await secondImport
    first.resolve(firstContents)
    await firstImport

    expect(workspace.importing).toBe(false)
    expect(workspace.editorText).toBe(secondContents)
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '已导入快照' })
  })

  it('忽略过期导入的成功结果并保留最新导入错误', async () => {
    const { workspace } = await mountWorkspace()
    const stale = deferred<string>()
    const staleContents = JSON.stringify({ ...fixture, releaseId: 'rel_134548aa-63f4-4733-ac15-af998adfd668' })
    const staleImport = workspace.importSnapshot({
      name: 'stale.json',
      size: staleContents.length,
      text: () => stale.promise,
    })

    await workspace.importSnapshot({
      name: 'latest.txt',
      size: 2,
      text: async () => '{}',
    }, 'en')
    stale.resolve(staleContents)
    await staleImport

    expect(workspace.editorText).toBe('{}')
    expect(workspace.workflow).toMatchObject({ status: 'error', message: 'Choose a JSON file' })
  })

  it('完成草稿、审核、发布和状态刷新流程', async () => {
    let currentVersion = version()
    let currentJob = publishJob()
    const publishKeys: string[] = []
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      const method = init?.method || 'GET'
      if (url.endsWith('/api/v1/versions') && method === 'POST') {
        return jsonResponse(currentVersion, 201)
      }
      if (url.endsWith('/api/v1/versions/ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d') && method === 'PUT') {
        currentVersion = version({
          revision: 2,
          snapshot: { ...structuredClone(fixture), releaseId: 'rel_d587e6bf-9f67-420a-8346-ee2da4c2458d' },
        })
        return jsonResponse(currentVersion)
      }
      if (url.endsWith('/review')) {
        currentVersion = version({ status: 'in_review', revision: 3 })
        return jsonResponse(currentVersion)
      }
      if (url.endsWith('/approve')) {
        currentVersion = version({ status: 'in_review', revision: 4, reviewApproved: true })
        return jsonResponse(currentVersion)
      }
      if (url.endsWith('/api/v1/publishes')) {
        publishKeys.push(new Headers(init?.headers).get('Idempotency-Key') || '')
        currentVersion = version({ status: 'publishing', revision: 5, reviewApproved: true })
        return jsonResponse(currentJob, 202)
      }
      if (url.endsWith('/refresh')) {
        currentJob = publishJob({ status: 'succeeded', buildId: 'build_12b823dc-69a5-4093-93ed-5a85034ddbea' })
        currentVersion = version({ status: 'published', revision: 6, reviewApproved: true })
        return jsonResponse(currentJob)
      }
      if (url.endsWith('/api/v1/versions/ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d') && method === 'GET') return jsonResponse(currentVersion)
      throw new Error(`unexpected request: ${method} ${url}`)
    })
    vi.stubGlobal('fetch', fetcher)
    const { workspace } = await mountWorkspace()

    workspace.editorText = JSON.stringify(fixture)
    await workspace.saveDraft()
    expect(workspace.canSubmitReview).toBe(true)

    workspace.editorText = JSON.stringify({ ...fixture, releaseId: 'rel_d587e6bf-9f67-420a-8346-ee2da4c2458d' })
    await workspace.saveDraft()
    expect(workspace.version?.revision).toBe(2)

    await workspace.submitReview()
    expect(workspace.canApprove).toBe(true)
    await workspace.approveReview()
    expect(workspace.canPublish).toBe(true)
    workspace.rejectReason = 'Too late to reject'
    const callsAfterApproval = fetcher.mock.calls.length
    await workspace.approveReview()
    await workspace.rejectReview()
    expect(fetcher.mock.calls).toHaveLength(callsAfterApproval)

    await workspace.publish()
    expect(workspace.version).toMatchObject({ status: 'publishing', revision: 5 })
    expect(workspace.canPublish).toBe(false)
    await workspace.publish()
    expect(publishKeys[1]).toBe(publishKeys[0])
    await workspace.refreshPublish()
    expect(workspace.publishJob).toMatchObject({ status: 'succeeded', buildId: 'build_12b823dc-69a5-4093-93ed-5a85034ddbea' })
    expect(workspace.version).toMatchObject({ status: 'published', revision: 6 })
    expect(workspace.versions[0]).toMatchObject({ status: 'published', revision: 6 })
    expect(workspace.canRollback).toBe(true)

    await workspace.publish()
    expect(publishKeys[2]).not.toBe(publishKeys[0])
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '发布任务已创建' })
  })

  it('载入、退回和回滚版本，并保留安全错误状态', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/versions/ver_d700307b-949c-4fb0-b56b-7e6d66348a68')) {
        return jsonResponse(version({ id: 'ver_d700307b-949c-4fb0-b56b-7e6d66348a68', status: 'in_review' }))
      }
      if (url.endsWith('/reject')) {
        return jsonResponse(version({ id: 'ver_d700307b-949c-4fb0-b56b-7e6d66348a68', revision: 2 }))
      }
      if (url.endsWith('/api/v1/rollbacks')) {
        return jsonResponse(publishJob({ id: 'rollback_1', versionId: 'ver_d700307b-949c-4fb0-b56b-7e6d66348a68', status: 'pending' }), 202)
      }
      return jsonResponse({ code: 'unavailable', message: '服务不可用', requestId: 'req_dfe7be6c-ee3b-49ca-a362-8de710b914dd' }, 503)
    })
    vi.stubGlobal('fetch', fetcher)
    const { workspace } = await mountWorkspace()

    workspace.versionId = 'ver_d700307b-949c-4fb0-b56b-7e6d66348a68'
    await workspace.loadVersion()
    expect(workspace.version?.status).toBe('in_review')

    await workspace.rejectReview()
    expect(workspace.workflow).toMatchObject({ status: 'error', message: '请填写退回原因' })
    workspace.rejectReason = '素材需更新'
    await workspace.rejectReview()
    expect(workspace.version?.status).toBe('draft')

    workspace.version = version({ id: 'ver_d700307b-949c-4fb0-b56b-7e6d66348a68', status: 'published' })
    expect(workspace.canRollback).toBe(true)
    await workspace.rollback()
    expect(workspace.publishJob).toMatchObject({ id: 'rollback_1', status: 'pending' })

    workspace.versionId = 'missing'
    await workspace.loadVersion()
    expect(workspace.workflow).toEqual({ status: 'error', message: '服务不可用', requestId: 'req_dfe7be6c-ee3b-49ca-a362-8de710b914dd' })
  })
})

it('retains an accepted task and its key when the version synchronization fails', async () => {
  let reads = 0
  const keys: string[] = []
  vi.stubGlobal('fetch', vi.fn(async (input, init) => {
    if (String(input).endsWith('/publishes')) {
      keys.push(new Headers(init?.headers).get('Idempotency-Key')!)
      return jsonResponse(publishJob())
    }
    return ++reads === 1 ? jsonResponse({ code: 'unavailable', message: 'Unavailable' }, 503)
      : jsonResponse(version({ status: 'publishing', revision: 3 }))
  }))
  const { wrapper, workspace } = await mountWorkspace()
  try {
    workspace.version = version({ status: 'in_review', reviewApproved: true })
    await workspace.publish()
    expect(workspace.publishJob?.id).toBe('pub_b2549aa9-b67e-4031-93fc-d0edfa9de14a')
    expect(workspace.workflow.status).toBe('error')
    await workspace.publish()
    expect(keys[1]).toBe(keys[0])
    expect(workspace.version?.status).toBe('publishing')
  } finally { wrapper.unmount() }
})

it('refreshes an old task summary without replacing the selected version or unsaved text', async () => {
  vi.stubGlobal('fetch', vi.fn(async input => String(input).endsWith('/refresh')
    ? jsonResponse(publishJob({ status: 'succeeded' }))
    : jsonResponse(version({ status: 'published', revision: 3 }))))
  const { wrapper, workspace } = await mountWorkspace()
  try {
    workspace.version = version({ id: 'ver_6e951bd5-bd9c-4f24-8450-4e9fe3c5ff49' })
    workspace.editorText = 'unsaved text'
    workspace.publishJob = publishJob()
    await workspace.refreshPublish()
    expect(workspace.version?.id).toBe('ver_6e951bd5-bd9c-4f24-8450-4e9fe3c5ff49')
    expect(workspace.editorText).toBe('unsaved text')
    expect(workspace.versions.find(item => item.id === 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d')).toMatchObject({ status: 'published', revision: 3 })
  } finally { wrapper.unmount() }
})

it.each([false, true])('synchronizes the previous live version after success, selected=%s', async (selectedPrevious) => {
  const previous = version({ id: 'ver_24580f8f-438f-44d4-bf71-f1cd7174ede4', status: 'published', revision: 8 })
  const target = version({ status: 'publishing', revision: 5 })
  vi.stubGlobal('fetch', vi.fn(async input => String(input).endsWith('/refresh')
    ? jsonResponse(publishJob({ status: 'succeeded' }))
    : String(input).endsWith('/ver_24580f8f-438f-44d4-bf71-f1cd7174ede4')
      ? jsonResponse({ ...previous, status: 'archived', revision: 9 })
      : jsonResponse({ ...target, status: 'published', revision: 6 })))
  const { wrapper, workspace } = await mountWorkspace()
  try {
    workspace.versions = [target, previous]
    workspace.version = selectedPrevious ? previous : target
    workspace.editorText = 'retain selected input'
    workspace.publishJob = publishJob()
    await workspace.refreshPublish()
    expect(workspace.versions.find(item => item.id === previous.id)).toMatchObject({ status: 'archived', revision: 9 })
    expect(workspace.version).toMatchObject(selectedPrevious ? { id: previous.id, status: 'archived', revision: 9 } : { id: target.id, status: 'published', revision: 6 })
    expect(workspace.editorText).toBe('retain selected input')
  } finally { wrapper.unmount() }
})

it('keeps a current dirty snapshot while refreshing its version status', async () => {
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce(jsonResponse(version({ status: 'publishing', revision: 4 })))
    .mockResolvedValueOnce(jsonResponse(publishJob({ status: 'failed' })))
    .mockResolvedValueOnce(jsonResponse(version({ status: 'in_review', revision: 5, reviewApproved: true }))))
  const { wrapper, workspace } = await mountWorkspace()
  try {
    workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
    await workspace.loadVersion()
    const changed = JSON.stringify({ ...fixture, releaseId: 'rel_e37f5566-d13a-46e9-ac61-cbb727c6a539' })
    workspace.editorText = changed
    workspace.publishJob = publishJob()
    await workspace.refreshPublish()
    expect(workspace.editorText).toBe(changed)
    expect(workspace.dirty).toBe(true)
    expect(workspace.version).toMatchObject({ status: 'in_review', revision: 5 })
    expect(workspace.canPublish).toBe(false)
  } finally { wrapper.unmount() }
})

it.each(['lost', 'conflict'] as const)('回滚请求 %s 后恢复服务端任务并继续跟踪', async (failure) => {
  const { wrapper, workspace } = await mountWorkspace()
  vi.useFakeTimers()
  const job = publishJob({ operation: 'rollback' })
  vi.stubGlobal('fetch', vi.fn(async input => {
    const url = String(input)
    if (url.endsWith('/rollbacks')) {
      if (failure === 'lost') throw new Error('Response lost')
      return jsonResponse({ code: 'conflict', message: 'Active job exists' }, 409)
    }
    if (url.endsWith('/latest')) return jsonResponse(job)
    if (url.endsWith(`/${job.id}`)) return jsonResponse({ ...job, status: 'succeeded' })
    return jsonResponse(version({ status: 'published', revision: 6 }))
  }))
  try {
    workspace.version = version({ status: 'archived' })
    await workspace.rollback()
    expect(workspace.workflow.status).toBe('error')
    expect(workspace.publishJob).toMatchObject({ id: job.id, status: 'building', operation: 'rollback' })
    await vi.advanceTimersByTimeAsync(5000)
    expect(workspace.publishJob?.status).toBe('succeeded')
    expect(workspace.version).toMatchObject({ status: 'published', revision: 6 })
  } finally { wrapper.unmount() }
})

it.each(['succeeded', 'failed'] as const)('synchronizes rollback acceptance and its %s terminal version', async (status) => {
  const finalStatus = status === 'succeeded' ? 'published' : 'archived'
  vi.stubGlobal('fetch', vi.fn()
    .mockResolvedValueOnce(jsonResponse(version({ status: 'archived', revision: 4 })))
    .mockResolvedValueOnce(jsonResponse(publishJob({ status: 'pending' }), 202))
    .mockResolvedValueOnce(jsonResponse(version({ status: 'publishing', revision: 5 })))
    .mockResolvedValueOnce(jsonResponse(publishJob({ status })))
    .mockResolvedValueOnce(jsonResponse(version({ status: finalStatus, revision: 6 }))))
  const { wrapper, workspace } = await mountWorkspace()
  try {
    workspace.versionId = 'ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d'
    await workspace.loadVersion()
    await workspace.rollback()
    expect(workspace.version).toMatchObject({ status: 'publishing', revision: 5 })
    expect(workspace.canRollback).toBe(false)
    await workspace.refreshPublish()
    expect(workspace.version).toMatchObject({ status: finalStatus, revision: 6 })
    expect(workspace.versions[0]).toMatchObject({ status: finalStatus, revision: 6 })
    expect(workspace.canRollback).toBe(true)
  } finally { wrapper.unmount() }
})

it('holds the write lock during version synchronization and ignores responses after session expiry', async () => {
  const active = ref(true)
  const read = deferred<Response>()
  vi.stubGlobal('fetch', vi.fn(async input => String(input).endsWith('/publishes')
    ? jsonResponse(publishJob()) : read.promise))
  const { wrapper, workspace } = await mountWorkspace(ref('en'), { active })
  try {
    workspace.version = version({ status: 'in_review', reviewApproved: true })
    const operation = workspace.publish()
    await vi.waitFor(() => expect(workspace.publishJob?.id).toBe('pub_b2549aa9-b67e-4031-93fc-d0edfa9de14a'))
    expect(workspace.busy).toBe(true)
    active.value = false
    read.resolve(jsonResponse(version({ status: 'published', revision: 3 })))
    await operation
    expect(workspace.version?.status).toBe('in_review')
    expect(workspace.workflow.status).toBe('idle')
  } finally { wrapper.unmount() }
})

it.each(['deployed', 'failed'] as const)('保留已接受的核实结果 %s，刷新失败只重试读取', async (phase) => {
  const { wrapper, workspace } = await mountWorkspace()
  vi.useFakeTimers()
  let recoveries = 0
  let reads = 0
  const recovered = { phase, deploymentId: phase === 'deployed' ? 'dp-verified' : '', error: phase === 'failed' ? 'Confirmed absent' : '' }
  const job = publishJob({ buildId: 'build_verified', build: { phase: 'submitting', error: 'Unknown outcome', configurationMismatch: phase === 'failed' } })
  const terminal = phase === 'deployed' ? 'succeeded' : 'failed'
  vi.stubGlobal('fetch', vi.fn(async (input) => {
    const url = String(input)
    if (url.endsWith('/recovery')) { recoveries++; return jsonResponse(recovered) }
    if (url.endsWith('/refresh')) return jsonResponse({ message: 'Unavailable' }, 503)
    if (url.endsWith(`/${job.id}`)) return ++reads === 1
      ? jsonResponse({ message: 'Unavailable' }, 503)
      : jsonResponse({ ...job, status: terminal, build: { ...job.build, ...recovered, phase: terminal } })
    return jsonResponse(version({ status: terminal === 'succeeded' ? 'published' : 'in_review' }))
  }))
  try {
    workspace.publishJob = job
    const accepted = await workspace.recoverPublish(phase === 'deployed' ? { note: 'Checked', deploymentId: 'dp-verified' } : { note: 'Checked', confirmedAbsent: true })
    expect(workspace.publishJob?.build).toMatchObject(recovered)
    expect(accepted).toBe(true)
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '核实结果已记录' })
    expect(workspace.publishReadError).toBe(true)
    expect(await workspace.recoverPublish({ note: 'Do not repeat', confirmedAbsent: true })).toBe(false)
    expect(recoveries).toBe(1)
    await vi.advanceTimersByTimeAsync(5000)
    expect(workspace.publishJob?.status).toBe(terminal)
    expect(workspace.publishReadError).toBe(false)
    expect(recoveries).toBe(1)
  } finally { wrapper.unmount() }
})

it.each(['expired', 'replaced', 'unmounted'] as const)('核实响应在上下文 %s 后到达时不写回', async (context) => {
  const active = ref(true)
  const response = deferred<Response>()
  const fetcher = vi.fn().mockReturnValue(response.promise)
  const { wrapper, workspace } = await mountWorkspace(ref('en'), { active })
  vi.stubGlobal('fetch', fetcher)
  const job = publishJob({ buildId: 'build_old', build: { phase: 'submitting', error: 'Unknown outcome', configurationMismatch: false } })
  workspace.publishJob = job
  try {
    const recovering = workspace.recoverPublish({ note: 'Checked', confirmedAbsent: true })
    if (context === 'expired') active.value = false
    else if (context === 'replaced') workspace.publishJob = { ...job, id: 'pub_new' }
    else wrapper.unmount()
    response.resolve(jsonResponse({ phase: 'failed', deploymentId: '', error: 'Confirmed absent' }))
    expect(await recovering).toBe(false)
    expect(workspace.publishJob?.build?.phase).toBe('submitting')
    expect(fetcher).toHaveBeenCalledTimes(1)
  } finally { if (context !== 'unmounted') wrapper.unmount() }
})

it('核实开始前的迟到状态读取不覆盖已经保存的结果', async () => {
  const { wrapper, workspace } = await mountWorkspace()
  const old = deferred<Response>()
  const job = publishJob({ buildId: 'build_verified', build: { phase: 'submitting', error: 'Unknown outcome', configurationMismatch: false } })
  let reads = 0
  vi.stubGlobal('fetch', vi.fn(async input => {
    if (String(input).endsWith('/recovery')) return jsonResponse({ phase: 'failed', deploymentId: '', error: 'Confirmed absent' })
    if (++reads === 1) return old.promise
    return jsonResponse({ message: 'Unavailable' }, 503)
  }))
  try {
    workspace.publishJob = job
    const reading = workspace.restorePublish()
    await workspace.recoverPublish({ note: 'Checked', confirmedAbsent: true })
    old.resolve(jsonResponse(job))
    await reading
    expect(workspace.publishJob?.build?.phase).toBe('failed')
  } finally { wrapper.unmount() }
})
