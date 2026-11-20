import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json'
import { eventCatalogMetadata, eventCatalogUrl, orderEvents } from '../../utils/event-catalog'

const snapshot = fixture as unknown as YujianContentSnapshot

describe('现场目录数据', () => {
  it('将未开始的演出按日期正序排列，再将历史与取消场次按日期倒序排列', () => {
    const data = structuredClone(snapshot)
    data.generatedAt = '2027-06-01T00:00:00Z'
    data.events.push(
      { ...data.events[0]!, id: 'event_past', dateTime: '2027-01-01T12:00:00Z' },
      { ...data.events[0]!, id: 'event_cancelled', dateTime: '2027-09-01T12:00:00Z', status: 'cancelled' },
      { ...data.events[0]!, id: 'event_sold_out', dateTime: '2027-06-15T12:00:00Z', status: 'sold-out' },
    )

    expect(orderEvents(data).map(event => event.id)).toEqual([
      'event_sold_out',
      'event_b9413813-c666-4987-bb0c-2850fd7fa156',
      'event_cancelled',
      'event_dc200c4c-abbb-476e-984d-405470f2dbea',
      'event_past',
    ])
  })

  it('按站点 canonical 生成稳定的目录 URL 与双语页面元数据', () => {
    expect(eventCatalogUrl('https://release.yujian.me')).toBe('https://release.yujian.me/events/')
    expect(eventCatalogMetadata(snapshot, 'zh-CN')).toMatchObject({
      title: expect.stringContaining('全部现场'),
      canonical: 'https://yujian.me/events/',
    })
    expect(eventCatalogMetadata(snapshot, 'en').title).toContain('Live events')
  })
})
