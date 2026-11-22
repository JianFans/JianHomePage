import type { YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import fixture from '../../../../content/fixtures/homepage.json'
import EventCatalog from '../../components/event/EventCatalog.vue'

const source = fixture as unknown as YujianContentSnapshot

it('完整呈现日期、外部详情和各场次状态', () => {
  const snapshot = structuredClone(source)
  snapshot.generatedAt = '2027-06-01T00:00:00Z'
  snapshot.events[0]!.dateTime = '2027-05-16T12:00:00Z'
  snapshot.events[1]!.status = 'sold-out'
  snapshot.events.push({ ...snapshot.events[0]!, id: 'event_cancelled', status: 'cancelled' })

  const wrapper = mount(EventCatalog, { props: { snapshot, locale: 'zh-CN' } })
  const cards = wrapper.findAll('[data-testid="event-card"]')

  expect(cards).toHaveLength(3)
  expect(cards[0]!.attributes('id')).toBe(snapshot.events[1]!.id)
  expect(cards[0]!.text()).toContain('已售罄')
  expect(cards[1]!.text()).toContain('已结束')
  expect(cards[1]!.get('time').text()).toBe('2027年5月16日 12:00 UTC')
  expect(cards[1]!.get('a').attributes()).toMatchObject({
    href: snapshot.events[0]!.detailUrl,
    target: '_blank',
    rel: 'noopener noreferrer',
  })
  expect(cards[2]!.text()).toContain('已取消')
})
