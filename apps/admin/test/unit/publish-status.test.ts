import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import PublishStatusPanel from '../../components/PublishStatusPanel.vue'
import type { AdminPublishJob } from '../../utils/admin-api'

/** 结果不明的任务必须核实，不能把创建请求直接重试。 */
function uncertainJob(): AdminPublishJob {
  return { id: 'pub_1', versionId: 'ver_1', status: 'building', buildId: 'build_1', operation: 'publish', snapshotKey: 'snapshots/1', snapshotChecksum: 'sha256:1', build: { phase: 'submitting', error: 'Deployment outcome unknown', configurationMismatch: false } }
}

describe('发布状态操作', () => {
  it('刷新指示与轮询状态分离，结束后恢复刷新按钮', async () => {
    const wrapper = mount(PublishStatusPanel, { props: { job: uncertainJob(), locale: 'en', busy: false, unsaved: false, canRecover: true, readError: false, refreshing: true } })
    try {
      expect(wrapper.get('[data-testid="publish-refresh"]').attributes('aria-busy')).toBe('true')
      expect(wrapper.get('[role="status"]').text()).toBe('Verification required')
      await wrapper.setProps({ refreshing: false })
      expect(wrapper.get('[data-testid="publish-refresh"]').attributes('aria-busy')).toBeUndefined()
    } finally { wrapper.unmount() }
  })
  it.each(['zh-CN', 'en'] as const)('已部署任务配置变化仍显示等待结果，不要求恢复配置（%s）', (locale) => {
    const wrapper = mount(PublishStatusPanel, { props: { job: { ...uncertainJob(), build: { phase: 'deployed', deploymentId: 'dp-original', configurationMismatch: true } }, locale, busy: false, unsaved: false, canRecover: true, readError: false, refreshing: false } })
    try {
      expect(wrapper.get('[role="status"]').text()).toBe(locale === 'en' ? 'Deployment accepted · checking result' : '部署已接受，等待结果')
      expect(wrapper.text()).toContain('dp-original')
      expect(wrapper.text()).not.toContain(locale === 'en' ? 'Restore its original configuration' : '恢复原配置')
      expect(wrapper.find('[data-testid="publish-recover"]').exists()).toBe(false)
    } finally { wrapper.unmount() }
  })

  it.each(['succeeded', 'failed'] as const)('已结束构建 %s 优先显示结果同步，不要求恢复旧配置', async (phase) => {
    const wrapper = mount(PublishStatusPanel, { props: { job: { ...uncertainJob(), build: { phase, configurationMismatch: true } }, locale: 'zh-CN', busy: false, unsaved: false, canRecover: true, readError: false, refreshing: false } })
    try {
      expect(wrapper.get('[role="status"]').text()).toBe('同步发布结果中')
      expect(wrapper.text()).not.toContain('恢复原配置')
      await wrapper.setProps({ job: { ...uncertainJob(), status: phase, build: { phase, configurationMismatch: true } } })
      expect(wrapper.get('[role="status"]').text()).toBe(phase === 'succeeded' ? '发布成功' : '发布失败')
      expect(wrapper.text()).not.toContain('恢复原配置')
    } finally { wrapper.unmount() }
  })

  it('显示真实阶段，限制恢复权限，并为终态失败提供单独确认的重试', async () => {
    const wrapper = mount(PublishStatusPanel, { props: { job: uncertainJob(), locale: 'en', busy: false, unsaved: false, canRecover: false, readError: false, refreshing: false } })
    expect(wrapper.text()).toContain('Verification required')
    expect(wrapper.find('[data-testid="publish-recover"]').exists()).toBe(false)
    await wrapper.setProps({ job: { ...uncertainJob(), status: 'failed', build: { phase: 'failed', error: 'Preparation failed', configurationMismatch: false } } })
    await wrapper.get('[data-testid="publish-retry"]').trigger('click')
    expect(wrapper.emitted('retry')).toBeUndefined()
    await wrapper.get('[data-testid="publish-confirm"]').trigger('click')
    expect(wrapper.emitted('retry')).toHaveLength(1)
    wrapper.unmount()
  })

  it('要求管理员明确核实；目标变化时只允许确认未创建，失败后保留输入', async () => {
    const wrapper = mount(PublishStatusPanel, { props: { job: { ...uncertainJob(), build: { ...uncertainJob().build!, configurationMismatch: true } }, locale: 'zh-CN', busy: false, unsaved: false, canRecover: true, readError: false, refreshing: false } })
    await wrapper.get('[data-testid="publish-recover"]').trigger('click')
    expect(wrapper.get('[data-testid="publish-confirm"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('option[value="existing"]').attributes('disabled')).toBeDefined()
    await wrapper.get('input[maxlength="500"]').setValue('已在原项目核实，没有部署')
    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.get('[data-testid="publish-confirm"]').trigger('click')
    expect(wrapper.emitted('recover')).toEqual([[{ note: '已在原项目核实，没有部署', confirmedAbsent: true }]])
    await wrapper.setProps({ busy: true })
    await wrapper.setProps({ busy: false })
    expect(wrapper.get<HTMLInputElement>('input[maxlength="500"]').element.value).toBe('已在原项目核实，没有部署')
    wrapper.unmount()
  })
})
