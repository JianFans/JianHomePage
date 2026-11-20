import type { YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import fixture from '../../../../content/fixtures/homepage.json'
import MomentCatalog from '../../components/moment/MomentCatalog.vue'

const source = fixture as unknown as YujianContentSnapshot

it('展示全量片段、说明及按目标类型解析的安全链接', () => {
  const snapshot = structuredClone(source)
  const originalCount = snapshot.moments.length
  snapshot.moments[0]!.target = { kind: 'external', link: { provider: 'website', url: 'https://example.com/moment' } }
  snapshot.moments.push(
    { ...snapshot.moments[0]!, id: 'moment_internal', target: { kind: 'internal', contentId: snapshot.events[0]!.id } },
    { ...snapshot.moments[0]!, id: 'moment_no_caption', caption: undefined, target: undefined },
  )
  snapshot.homepage.sections.find(section => section.type === 'moment')!.enabled = false

  const wrapper = mount(MomentCatalog, { props: { snapshot, locale: 'zh-CN' } })
  const cards = wrapper.findAll('[data-testid="moment-card"]')

  expect(cards).toHaveLength(snapshot.moments.length)
  expect(cards.map(card => card.attributes('id'))).toEqual(snapshot.moments.map(moment => moment.id))
  expect(cards[0]!.text()).toContain(snapshot.moments[0]!.caption!['zh-CN'])
  expect(cards[0]!.get('a').attributes()).toMatchObject({
    href: snapshot.moments[0]!.target!.kind === 'external' ? snapshot.moments[0]!.target.link.url : '',
    target: '_blank',
    rel: 'noopener noreferrer',
  })
  expect(cards[originalCount]!.get('a').attributes('href')).toBe(`/events/#${snapshot.events[0]!.id}`)
  expect(cards[originalCount + 1]!.find('a').exists()).toBe(false)
  expect(cards[originalCount + 1]!.find('figcaption').exists()).toBe(false)
})

it('空快照显示本地化空状态', () => {
  const snapshot = structuredClone(source)
  snapshot.moments = []

  const wrapper = mount(MomentCatalog, { props: { snapshot, locale: 'en' } })

  expect(wrapper.text()).toContain('No moments yet')
  expect(wrapper.findAll('[data-testid="moment-card"]')).toHaveLength(0)
})
