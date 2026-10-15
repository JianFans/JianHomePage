import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import LocalizedTextFields from '../../components/LocalizedTextFields.vue'

describe('双语字段', () => {
  it('中文界面默认折叠英文，切换英文后展开且清空英文会移除可选键', async () => {
    const value = { 'zh-CN': '标题', en: 'Title' }
    const wrapper = mount(LocalizedTextFields, { props: { modelValue: value, label: 'Title', locale: 'zh-CN', testId: 'title' } })
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.get('summary').text()).toContain('English')
    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('details').attributes('open')).toBeDefined()
    await wrapper.get('[data-testid="title-en"]').setValue('')
    expect(value).not.toHaveProperty('en')
    expect(value['zh-CN']).toBe('标题')
    wrapper.unmount()
  })
})
