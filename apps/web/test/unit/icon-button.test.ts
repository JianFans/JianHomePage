import { mount } from '@vue/test-utils'
import IconButton from '../../components/ui/IconButton.vue'

describe('IconButton', () => {
  it('提供可访问名称和关联的悬浮说明', () => {
    const wrapper = mount(IconButton, {
      props: { label: '播放' },
      slots: { default: '<span aria-hidden="true">P</span>' },
    })

    const button = wrapper.get('button')
    const tooltip = wrapper.get('[role="tooltip"]')
    expect(button.attributes('type')).toBe('button')
    expect(button.attributes('aria-label')).toBe('播放')
    expect(button.attributes('aria-describedby')).toBe(tooltip.attributes('id'))
    expect(tooltip.text()).toBe('播放')
  })

  it('禁用时不发送点击事件', async () => {
    const wrapper = mount(IconButton, {
      props: { label: '下一项', disabled: true },
    })

    await wrapper.get('button').trigger('click')
    expect(wrapper.emitted('click')).toBeUndefined()
  })

  it('支持在窄屏边缘向内对齐悬浮说明', () => {
    const wrapper = mount(IconButton, {
      props: { label: '切换语言', tooltipAlign: 'end' },
    })

    expect(wrapper.get('[role="tooltip"]').classes()).toContain('icon-button-tooltip--end')
  })
})
