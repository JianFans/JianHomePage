import { describe, expect, it } from 'vitest'
import { idleWorkflow, workflowError, workflowSuccess } from '../../utils/admin-workflow'

describe('admin workflow state', () => {
  it('starts idle without stale request metadata', () => {
    expect(idleWorkflow()).toEqual({ status: 'idle', message: '', requestId: '' })
  })

  it('keeps a request id while surfacing an API error', () => {
    expect(workflowError({ message: '冲突', requestId: 'req-1' })).toEqual({
      status: 'error',
      message: '冲突',
      requestId: 'req-1',
    })
  })

  it('represents a successful action without retaining old request ids', () => {
    expect(workflowSuccess('已保存')).toEqual({ status: 'success', message: '已保存', requestId: '' })
  })
})
