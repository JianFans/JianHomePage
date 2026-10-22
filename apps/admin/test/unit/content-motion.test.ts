import { mount } from '@vue/test-utils'
import { afterEach, expect, it, vi } from 'vitest'
import { vContentMotion } from '../../utils/content-motion'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

it('记录切换保留 DOM 和输入，替换旧动画并清理偏好监听', async () => {
  let reduced!: (event: MediaQueryListEvent) => void
  const remove = vi.fn()
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: false, addEventListener: (_: string, listener: typeof reduced) => { reduced = listener }, removeEventListener: remove })))
  const cancel = vi.fn()
  const animate = vi.spyOn(HTMLElement.prototype, 'animate').mockReturnValue({ cancel, finished: new Promise<void>(() => {}) } as unknown as Animation)
  const wrapper = mount({ directives: { contentMotion: vContentMotion }, data: () => ({ record: 'a', text: 'input retained' }), template: '<fieldset v-content-motion="record"><input v-model="text"></fieldset>' })
  const input = wrapper.get('input').element
  expect(animate).toHaveBeenCalledTimes(1)
  await wrapper.setData({ record: 'b' })
  expect(animate).toHaveBeenCalledTimes(2)
  expect(wrapper.get('input').element).toBe(input)
  expect(input.value).toBe('input retained')
  await wrapper.setData({ record: 'c' })
  expect(cancel).toHaveBeenCalledTimes(2)
  reduced({ matches: true } as MediaQueryListEvent)
  expect(cancel).toHaveBeenCalledTimes(3)
  wrapper.unmount()
  expect(remove).toHaveBeenCalledWith('change', reduced)
})

it('keyed 记录重建字段集时，新节点仍启动真实动效', async () => {
  const wrapper = mount({ directives: { contentMotion: vContentMotion }, data: () => ({ record: 'a' }), template: '<fieldset :key="record" v-content-motion="record"><input :value="record"></fieldset>' })
  try {
    const previous = wrapper.get('fieldset').element
    expect(previous.getAnimations().some(animation => animation.id === 'studio-record-change')).toBe(true)
    await wrapper.setData({ record: 'b' })
    const current = wrapper.get('fieldset').element
    expect(current).not.toBe(previous)
    expect(current.getAnimations().some(animation => animation.id === 'studio-record-change')).toBe(true)
    expect(wrapper.get('input').element.value).toBe('b')
  } finally { wrapper.unmount() }
})

it('减少动态效果时直接呈现新内容', async () => {
  vi.stubGlobal('matchMedia', vi.fn(() => ({ matches: true, addEventListener: vi.fn(), removeEventListener: vi.fn() })))
  const animate = vi.spyOn(HTMLElement.prototype, 'animate')
  const wrapper = mount({ directives: { contentMotion: vContentMotion }, data: () => ({ record: 'a' }), template: '<div v-content-motion="record">{{ record }}</div>' })
  await wrapper.setData({ record: 'b' })
  expect(wrapper.text()).toBe('b')
  expect(animate).not.toHaveBeenCalled()
  wrapper.unmount()
})

it('浏览器不支持 animate 时仍可更新内容', async () => {
  const descriptor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'animate')
  Object.defineProperty(HTMLElement.prototype, 'animate', { configurable: true, value: undefined })
  try {
    const wrapper = mount({ directives: { contentMotion: vContentMotion }, data: () => ({ record: 'a' }), template: '<div v-content-motion="record">{{ record }}</div>' })
    await wrapper.setData({ record: 'b' })
    expect(wrapper.text()).toBe('b')
    wrapper.unmount()
  } finally {
    if (descriptor) Object.defineProperty(HTMLElement.prototype, 'animate', descriptor)
    else Reflect.deleteProperty(HTMLElement.prototype, 'animate')
  }
})

it('真实动画连续取消和卸载不会留下未处理的 finished 拒绝', async () => {
  const wrapper = mount({ directives: { contentMotion: vContentMotion }, data: () => ({ record: 'a' }), template: '<div v-content-motion="record">{{ record }}</div>' })
  await wrapper.setData({ record: 'b' })
  await wrapper.setData({ record: 'c' })
  expect(wrapper.text()).toBe('c')
  wrapper.unmount()
})
