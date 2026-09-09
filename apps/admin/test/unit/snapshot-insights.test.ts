import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import SnapshotInsights from '../../components/SnapshotInsights.vue'
import type { SnapshotAnalysis } from '../../utils/snapshot-workbench'

const summaryAnalysis: SnapshotAnalysis = {
  snapshot: null,
  issues: [],
  summary: {
    sections: 6,
    releases: 5,
    tracks: 5,
    videos: 3,
    events: 2,
    moments: 3,
    assets: 15,
    previews: 3,
  },
}

describe('快照摘要与诊断', () => {
  it('展示中文和英文内容摘要', async () => {
    const wrapper = mount(SnapshotInsights, {
      props: { analysis: summaryAnalysis, locale: 'zh-CN' },
    })

    expect(wrapper.get('[data-testid="snapshot-summary"]').text()).toContain('素材15')

    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('[data-testid="snapshot-summary"]').text()).toContain('Assets15')
  })

  it('最多展示八条诊断并标明剩余数量', () => {
    const analysis: SnapshotAnalysis = {
      snapshot: null,
      summary: null,
      issues: Array.from({ length: 10 }, (_, index) => ({
        path: `/assets/${index}/id`,
        source: 'semantic' as const,
        code: index === 0 ? 'duplicate-id' : 'asset-kind',
      })),
    }
    const wrapper = mount(SnapshotInsights, {
      props: { analysis, locale: 'zh-CN' },
    })

    expect(wrapper.get('[data-testid="snapshot-issues"]').text()).toContain('/assets/0/id')
    expect(wrapper.get('[data-testid="snapshot-issues"]').text()).toContain('标识重复')
    expect(wrapper.findAll('[data-testid="snapshot-issue"]')).toHaveLength(8)
    expect(wrapper.text()).toContain('另有 2 项')
  })

  it('展示诊断来源和常见数值约束', () => {
    const wrapper = mount(SnapshotInsights, {
      props: {
        locale: 'zh-CN',
        analysis: {
          snapshot: null,
          summary: null,
          issues: [{ path: '/homepage/sections/1/limit', source: 'schema', code: 'minimum' }],
        },
      },
    })

    expect(wrapper.get('[data-testid="snapshot-issue-source"]').text()).toBe('Schema')
    expect(wrapper.get('[data-testid="snapshot-issue"]').text()).toContain('数值过小')
  })

  it('本地化严格最小值约束', async () => {
    const wrapper = mount(SnapshotInsights, {
      props: {
        locale: 'zh-CN',
        analysis: {
          snapshot: null,
          summary: null,
          issues: [{ path: '/tracks/0/durationSeconds', source: 'schema', code: 'exclusiveMinimum' }],
        },
      },
    })

    expect(wrapper.get('[data-testid="snapshot-issue"]').text()).toContain('数值必须大于下限')

    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('[data-testid="snapshot-issue"]').text()).toContain('Value must be above minimum')
  })

  it('本地化数组重复约束', async () => {
    const wrapper = mount(SnapshotInsights, {
      props: {
        locale: 'zh-CN',
        analysis: {
          snapshot: null,
          summary: null,
          issues: [{ path: '/site/supportedLocales', source: 'schema', code: 'uniqueItems' }],
        },
      },
    })

    expect(wrapper.get('[data-testid="snapshot-issue"]').text()).toContain('项目重复')

    await wrapper.setProps({ locale: 'en' })
    expect(wrapper.get('[data-testid="snapshot-issue"]').text()).toContain('Duplicate items')
  })
})
