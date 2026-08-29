import type { AdminApiError } from './admin-api'

export type WorkflowStatus = 'idle' | 'loading' | 'saving' | 'reviewing' | 'publishing' | 'success' | 'error'

export interface WorkflowState {
  status: WorkflowStatus
  message: string
  requestId: string
}

export function idleWorkflow(): WorkflowState {
  return { status: 'idle', message: '', requestId: '' }
}

export function workflowError(error: unknown): WorkflowState {
  const apiError = error as Partial<AdminApiError>
  return {
    status: 'error',
    message: apiError.message || '操作失败，请稍后重试。',
    requestId: apiError.requestId || '',
  }
}

export function workflowSuccess(message: string): WorkflowState {
  return { status: 'success', message, requestId: '' }
}
