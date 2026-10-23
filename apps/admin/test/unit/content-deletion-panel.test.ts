import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import ContentDeletionPanel from '../../components/ContentDeletionPanel.vue'
import type { YujianContentSnapshot } from '@yujian/schema'

const text = JSON.stringify(fixture)
const props = { text, analyzedText: text, snapshot: fixture as unknown as YujianContentSnapshot, kind: 'video' as const, locale: 'zh-CN' as const, locked: false }

describe('删除确认面板', () => {
  it('等待共享分析追上当前文本后才允许删除，不使用防抖前的记录', async () => {
    const wrapper = mount(ContentDeletionPanel, { props: { ...props, text: text + '\n' } })
    try {
      expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeDefined()
      await wrapper.setProps({ analyzedText: text + '\n' })
      expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeUndefined()
      await wrapper.setProps({ text: '{' })
      expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeDefined()
      expect(wrapper.emitted('update:text')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it('显示当前类别的记录和影响路径，明确勾选后才提交快照修改', async () => {
    const wrapper = mount(ContentDeletionPanel, { props })
    try {
      expect(wrapper.get('[data-testid="content-delete-select"]').findAll('option')).toHaveLength(3)
      await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
      expect(wrapper.get('[role="dialog"]').text()).toContain('/videos/0')
      expect(wrapper.get('[role="dialog"]').text()).toContain('/homepage/sections/2/itemIds/0')
      expect(wrapper.get('[data-testid="content-delete-confirm"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="content-delete-ack"]').setValue(true)
      await wrapper.get('[data-testid="content-delete-confirm"]').trigger('click')
      const output = wrapper.emitted('update:text')![0]![0] as string
      expect(JSON.parse(output).videos.map((record: { id: string }) => record.id)).not.toContain(fixture.videos[0]!.id)
      expect(wrapper.emitted('notify')![0]![1]).toBe('success')
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
    } finally { wrapper.unmount() }
  })

  it('阻塞引用逐条显示并禁用确认，不修改原文', async () => {
    const wrapper = mount(ContentDeletionPanel, { props: { ...props, kind: 'release' } })
    try {
      await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
      expect(wrapper.get('[role="dialog"]').text()).toContain('/heroSlides/2/releaseId')
      expect(wrapper.get('[role="dialog"]').text()).toContain('/heroSlides/2/target/contentId')
      await wrapper.get('[data-testid="content-delete-ack"]').setValue(true)
      expect(wrapper.get('[data-testid="content-delete-confirm"]').attributes('disabled')).toBeDefined()
      await wrapper.get('[data-testid="content-delete-confirm"]').trigger('click')
      expect(wrapper.emitted('update:text')).toBeUndefined()
    } finally { wrapper.unmount() }
  })

  it.each(['text', 'locked', 'kind'] as const)('确认期间 %s 改变使确认失效', async (key) => {
    const wrapper = mount(ContentDeletionPanel, { props })
    try {
      await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
      await wrapper.get('[data-testid="content-delete-ack"]').setValue(true)
      await wrapper.setProps(key === 'text' ? { text: `${text}\n`, analyzedText: `${text}\n` } : key === 'locked' ? { locked: true } : { kind: 'event' })
      expect(wrapper.find('[role="dialog"]').exists()).toBe(false)
      expect(wrapper.emitted('update:text')).toBeUndefined()
      if (key === 'locked') expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeDefined()
      else {
        await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
        expect((wrapper.get('[data-testid="content-delete-ack"]').element as HTMLInputElement).checked).toBe(false)
      }
    } finally { wrapper.unmount() }
  })

  it('切换删除记录只影响被确认目标，并随语言切换提供英文操作', async () => {
    const wrapper = mount(ContentDeletionPanel, { props: { ...props, locale: 'en' } })
    try {
      expect(wrapper.get('[data-testid="content-delete-open"]').text()).toContain('Delete')
      await wrapper.get('[data-testid="content-delete-select"]').setValue(fixture.videos[1]!.id)
      await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
      expect(wrapper.get('[role="dialog"]').text()).toContain('Sample Video 02')
      await wrapper.get('[data-testid="content-delete-ack"]').setValue(true)
      await wrapper.get('[data-testid="content-delete-confirm"]').trigger('click')
      const output = JSON.parse(wrapper.emitted('update:text')![0]![0] as string)
      expect(output.videos[0].id).toBe(fixture.videos[0]!.id)
      expect(output.videos).toHaveLength(2)
    } finally { wrapper.unmount() }
  })

  it('锁定、非法快照和最后一首曲目不可删除', async () => {
    const wrapper = mount(ContentDeletionPanel, { props: { ...props, locked: true } })
    try {
      expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeDefined()
      await wrapper.setProps({ locked: false, text: '{', analyzedText: '{', snapshot: null })
      expect(wrapper.get('[data-testid="content-delete-open"]').attributes('disabled')).toBeDefined()
      expect(wrapper.emitted('update:text')).toBeUndefined()
      expect(wrapper.get('[role="status"]').text()).toContain('请先修正快照校验问题')
      await wrapper.setProps({ text, analyzedText: text, snapshot: props.snapshot, kind: 'track' })
      await wrapper.get('[data-testid="content-delete-open"]').trigger('click')
      expect(wrapper.get('[data-testid="content-delete-confirm"]').attributes('disabled')).toBeDefined()
      expect(wrapper.get('[role="dialog"]').text()).toContain('最后一首')
    } finally { wrapper.unmount() }
  })
})
