import { mountSuspended } from '@nuxt/test-utils/runtime'
import { defineComponent, reactive } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'
import fixtureData from '../../../../content/fixtures/homepage.json'
import { useAdminWorkspace } from '../../composables/useAdminWorkspace'
import type { AdminPublishJob, AdminVersion } from '../../utils/admin-api'

const fixture = fixtureData as unknown as Record<string, unknown>

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

async function mountWorkspace() {
  const host = defineComponent({
    setup() {
      return { workspace: reactive(useAdminWorkspace()) }
    },
    template: '<div />',
  })
  const wrapper = await mountSuspended(host)
  return { wrapper, workspace: wrapper.vm.workspace }
}

function version(overrides: Partial<AdminVersion> = {}): AdminVersion {
  return {
    id: 'ver_1',
    status: 'draft',
    revision: 1,
    snapshot: structuredClone(fixture),
    checksum: 'sha256:version',
    ...overrides,
  }
}

function publishJob(overrides: Partial<AdminPublishJob> = {}): AdminPublishJob {
  return {
    id: 'pub_1',
    versionId: 'ver_1',
    status: 'building',
    snapshotKey: 'snapshots/ver_1.json',
    snapshotChecksum: 'sha256:version',
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('管理工作区', () => {
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
    const contents = JSON.stringify(fixture)

    workspace.editorText = contents
    expect(workspace.editorAnalysis.issues).toHaveLength(0)
    expect(workspace.canSave).toBe(true)
    expect(workspace.exportSnapshot()).toMatchObject({
      filename: 'rel_fixture_20260829.json',
      mimeType: 'application/json',
    })

    workspace.editorText = '{}'
    expect(workspace.canSave).toBe(false)
    expect(workspace.exportSnapshot()).toBeNull()

    await workspace.importSnapshot({
      name: 'draft.json',
      size: contents.length,
      text: async () => contents,
    })
    expect(workspace.editorAnalysis.snapshot?.releaseId).toBe('rel_fixture_20260829')
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '已导入快照' })
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
      if (url.endsWith('/api/v1/versions/ver_1') && method === 'PUT') {
        currentVersion = version({
          revision: 2,
          snapshot: { ...structuredClone(fixture), releaseId: 'rel_fixture_revision_2' },
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
        return jsonResponse(currentJob, 202)
      }
      if (url.endsWith('/refresh')) {
        currentJob = publishJob({ status: 'succeeded', buildId: 'build_1' })
        return jsonResponse(currentJob)
      }
      throw new Error(`unexpected request: ${method} ${url}`)
    })
    vi.stubGlobal('fetch', fetcher)
    const { workspace } = await mountWorkspace()

    workspace.editorText = JSON.stringify(fixture)
    await workspace.saveDraft()
    expect(workspace.canSubmitReview).toBe(true)

    workspace.editorText = JSON.stringify({ ...fixture, releaseId: 'rel_fixture_revision_2' })
    await workspace.saveDraft()
    expect(workspace.version?.revision).toBe(2)

    await workspace.submitReview()
    expect(workspace.canApprove).toBe(true)
    await workspace.approveReview()
    expect(workspace.canPublish).toBe(true)

    await workspace.publish()
    await workspace.publish()
    expect(publishKeys[1]).toBe(publishKeys[0])
    await workspace.refreshPublish()
    expect(workspace.publishJob).toMatchObject({ status: 'succeeded', buildId: 'build_1' })

    await workspace.publish()
    expect(publishKeys[2]).not.toBe(publishKeys[0])
    expect(workspace.workflow).toMatchObject({ status: 'success', message: '发布任务已创建' })
  })

  it('载入、退回和回滚版本，并保留安全错误状态', async () => {
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/versions/ver_load')) {
        return jsonResponse(version({ id: 'ver_load', status: 'in_review' }))
      }
      if (url.endsWith('/reject')) {
        return jsonResponse(version({ id: 'ver_load', revision: 2 }))
      }
      if (url.endsWith('/api/v1/rollbacks')) {
        return jsonResponse(publishJob({ id: 'rollback_1', versionId: 'ver_load', status: 'pending' }), 202)
      }
      return jsonResponse({ code: 'unavailable', message: '服务不可用', requestId: 'req_1' }, 503)
    })
    vi.stubGlobal('fetch', fetcher)
    const { workspace } = await mountWorkspace()

    workspace.versionId = 'ver_load'
    await workspace.loadVersion()
    expect(workspace.version?.status).toBe('in_review')

    await workspace.rejectReview()
    expect(workspace.workflow).toMatchObject({ status: 'error', message: '请填写退回原因' })
    workspace.rejectReason = '素材需更新'
    await workspace.rejectReview()
    expect(workspace.version?.status).toBe('draft')

    workspace.version = version({ id: 'ver_load', status: 'published' })
    expect(workspace.canRollback).toBe(true)
    await workspace.rollback()
    expect(workspace.publishJob).toMatchObject({ id: 'rollback_1', status: 'pending' })

    workspace.versionId = 'missing'
    await workspace.loadVersion()
    expect(workspace.workflow).toEqual({ status: 'error', message: '服务不可用', requestId: 'req_1' })
  })
})
