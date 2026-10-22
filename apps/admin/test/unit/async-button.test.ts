import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import AsyncButton from '../../components/AsyncButton.vue'

describe('异步按钮', () => {
  it('保留原按钮内容及属性，加载时阻止重复点击并立即恢复', async () => {
    const clicked = vi.fn()
    const wrapper = mount(AsyncButton, { props: { loading: false, onClick: clicked }, attrs: { 'aria-label': '保存草稿', 'data-testid': 'save', class: 'button' }, slots: { default: '<span>保存草稿</span>' } })
    expect(wrapper.attributes('type')).toBe('button')
    expect(wrapper.attributes('aria-label')).toBe('保存草稿')
    expect(wrapper.classes()).toContain('button')
    await wrapper.trigger('click')
    expect(clicked).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ loading: true })
    expect(wrapper.attributes('aria-busy')).toBe('true')
    expect(wrapper.element.matches(':disabled')).toBe(true)
    expect(wrapper.text()).toBe('保存草稿')
    expect(wrapper.find('.async-button__content').exists()).toBe(true)
    await wrapper.trigger('click')
    expect(clicked).toHaveBeenCalledTimes(1)
    await wrapper.setProps({ loading: false })
    expect(wrapper.element.matches(':disabled')).toBe(false)
    await wrapper.trigger('click')
    expect(clicked).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('外部 disabled 在 loading 结束后仍生效，并支持原生 submit 类型', async () => {
    const wrapper = mount(AsyncButton, { props: { disabled: true, loading: true, type: 'submit' }, slots: { default: '上传' } })
    await wrapper.setProps({ loading: false })
    expect(wrapper.attributes('type')).toBe('submit')
    expect(wrapper.element.matches(':disabled')).toBe(true)
    expect(wrapper.attributes('aria-busy')).toBeUndefined()
    wrapper.unmount()
  })
})
