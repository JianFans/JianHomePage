import { nextTick } from 'vue'
import type { AdminLocale } from './admin-locale'
import type { SnapshotEditorIssue } from './snapshot-workbench'

/** Translate stable schema and semantic diagnostic codes consistently across the editor. */
export function diagnosticLabel(code: string, locale: AdminLocale): string {
  const labels: Record<string, { zh: string; en: string }> = {
    'invalid-json': { zh: 'JSON 格式无效', en: 'Invalid JSON' },
    'object-root': { zh: '根节点必须为对象', en: 'Root must be an object' },
    additionalProperties: { zh: '存在不允许的字段', en: 'Field is not allowed' },
    required: { zh: '缺少必填字段', en: 'Required field missing' },
    type: { zh: '字段类型错误', en: 'Incorrect field type' },
    format: { zh: '字段格式错误', en: 'Incorrect value format' },
    pattern: { zh: '字段格式错误', en: 'Incorrect value format' },
    minimum: { zh: '数值过小', en: 'Value below minimum' },
    exclusiveMinimum: { zh: '数值必须大于下限', en: 'Value must be above minimum' },
    maximum: { zh: '数值过大', en: 'Value above maximum' },
    minLength: { zh: '文本过短', en: 'Text is too short' },
    maxLength: { zh: '文本过长', en: 'Text is too long' },
    minItems: { zh: '项目过少', en: 'Too few items' },
    maxItems: { zh: '项目过多', en: 'Too many items' },
    uniqueItems: { zh: '项目重复', en: 'Duplicate items' },
    enum: { zh: '不支持的值', en: 'Unsupported value' },
    const: { zh: '值与内容类型不匹配', en: 'Unexpected value' },
    oneOf: { zh: '值不符合支持的结构', en: 'Value does not match a supported form' },
    discriminator: { zh: '不支持的内容类型', en: 'Unsupported content type' },
    'duplicate-id': { zh: '标识重复', en: 'Duplicate identifier' },
    'missing-reference': { zh: '引用不存在', en: 'Reference not found' },
    'reference-mismatch': { zh: '引用归属不一致', en: 'Reference belongs elsewhere' },
    'asset-kind': { zh: '素材类型不匹配', en: 'Asset type mismatch' },
    'hidden-target': { zh: '目标未在首页展示', en: 'Target is not visible' },
  }
  const label = labels[code]
  const summary = label ? (locale === 'en' ? label.en : label.zh) : (locale === 'en' ? 'Content contract mismatch' : '内容契约不匹配')
  return summary
}

/** Add a stable JSON Pointer to the localized field-level diagnostic. */
export function issueDescription(issue: SnapshotEditorIssue, locale: AdminLocale): string {
  return `${diagnosticLabel(issue.code, locale)} · ${issue.path}`
}

/** Bind a control to its exact canonical pointer and localized issue hint. */
export function issueFieldAttributes(issue: SnapshotEditorIssue | undefined, path: string, id: string, locale: AdminLocale): {
  'data-snapshot-path': string
  'aria-invalid': 'true' | undefined
  'aria-describedby': string | undefined
  description: string
} {
  return issue
    ? { 'data-snapshot-path': path, 'aria-invalid': 'true', 'aria-describedby': id, description: issueDescription(issue, locale) }
    : { 'data-snapshot-path': path, 'aria-invalid': undefined, 'aria-describedby': undefined, description: '' }
}

/** Select the record named by a JSON Pointer, expand its field group, then focus it. */
export async function focusSnapshotIssue(root: ParentNode, issues: readonly SnapshotEditorIssue[]): Promise<boolean> {
  const issue = issues[0]
  if (!issue) return false
  const path = issue.path
  const match = /^\/(heroSlides|releases|tracks|videos|events|moments)\/(\d+)(?:\/|$)/.exec(path)
  if (match) {
    const [, kind, index] = match
    const record = [...root.querySelectorAll<HTMLElement>('[data-snapshot-record-kind][data-snapshot-record-index]')]
      .find(element => element.dataset.snapshotRecordKind === kind && element.dataset.snapshotRecordIndex === index)
    if (record && !record.matches('[aria-pressed="true"]')) {
      record.click()
      await nextTick()
    }
  }
  const field = [...root.querySelectorAll<HTMLElement>('[data-snapshot-path]')]
    .find(element => element.dataset.snapshotPath === path)
  if (!field) return false
  let expanded = false
  for (let ancestor = field.parentElement; ancestor; ancestor = ancestor.parentElement) {
    if (ancestor.tagName === 'DETAILS' && !(ancestor as HTMLDetailsElement).open) {
      (ancestor as HTMLDetailsElement).open = true
      expanded = true
    }
  }
  // details 的可聚焦子树在下一帧才更新；等可见布局，不等待任何入场动画。
  if (expanded) await new Promise<void>(resolve => requestAnimationFrame(() => resolve()))
  if (!field.isConnected || field.dataset.snapshotPath !== path) return false
  field.focus()
  const reduceMotion = typeof window !== 'undefined' && window.matchMedia?.('(prefers-reduced-motion: reduce)').matches
  field.scrollIntoView?.({ block: 'center', behavior: reduceMotion ? 'instant' : 'auto' })
  return true
}
