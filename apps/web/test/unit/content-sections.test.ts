import type { YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import fixture from '../../../../content/fixtures/homepage.json'
import ArtistSection from '../../components/home/ArtistSection.vue'
import EventSection from '../../components/home/EventSection.vue'
import MomentSection from '../../components/home/MomentSection.vue'
import VideoSection from '../../components/home/VideoSection.vue'

const snapshot = fixture as unknown as YujianContentSnapshot

describe('VideoSection', () => {
  it('以一大两小布局渲染安全影像入口', () => {
    const wrapper = mount(VideoSection, {
      props: {
        items: snapshot.videos,
        assets: snapshot.assets,
        locale: 'zh-CN',
      },
    })
    const cards = wrapper.findAll('[data-testid="video-card"]')

    expect(cards).toHaveLength(3)
    expect(cards[0]!.classes()).toContain('video-card--featured')
    expect(cards[0]!.get('img').attributes('loading')).toBe('lazy')
    expect(cards[0]!.attributes()).toMatchObject({
      id: snapshot.videos[0]!.id,
      target: '_blank',
      rel: 'noopener noreferrer',
    })
  })
})

describe('EventSection', () => {
  it('以语义日期线呈现未来现场', () => {
    const wrapper = mount(EventSection, {
      props: {
        items: snapshot.events,
        locale: 'zh-CN',
      },
    })
    const events = wrapper.findAll('[data-testid="event-item"]')

    expect(events).toHaveLength(2)
    expect(events[0]!.attributes('id')).toBe(snapshot.events[0]!.id)
    expect(events[0]!.get('time').attributes('datetime')).toBe(snapshot.events[0]!.dateTime)
    expect(events[0]!.get('a').attributes('href')).toBe(snapshot.events[0]!.detailUrl)
  })
})

describe('MomentSection', () => {
  it('使用无可见说明文字的稳定比例图片拼贴', () => {
    const wrapper = mount(MomentSection, {
      props: {
        items: snapshot.moments,
        assets: snapshot.assets,
        locale: 'zh-CN',
      },
    })
    const items = wrapper.findAll('[data-testid="moment-item"]')

    expect(items).toHaveLength(3)
    expect(items.map(item => item.attributes('id'))).toEqual(snapshot.moments.map(moment => moment.id))
    expect(items.every(item => item.get('img').attributes('loading') === 'lazy')).toBe(true)
    expect(wrapper.findAll('.moment-caption--visible')).toHaveLength(0)
  })

  it('将 Moment 的外部和内部目标渲染为安全链接', () => {
    const moments = structuredClone(snapshot.moments)
    moments[0]!.target = {
      kind: 'external',
      link: { provider: 'bilibili', url: 'https://space.bilibili.com/21096618' },
    }
    moments[1]!.target = { kind: 'internal', contentId: 'video_01' }
    const wrapper = mount(MomentSection, {
      props: { items: moments, assets: snapshot.assets, locale: 'zh-CN' },
    })
    const items = wrapper.findAll('[data-testid="moment-item"]')

    expect(items[0]!.get('a').attributes()).toMatchObject({
      href: 'https://space.bilibili.com/21096618',
      target: '_blank',
      rel: 'noopener noreferrer',
    })
    expect(items[1]!.get('a').attributes('href')).toBe('#video_01')
    expect(items[1]!.get('a').attributes('target')).toBeUndefined()
  })
})

describe('ArtistSection', () => {
  it('只保留姓名、短简介和官方平台入口', () => {
    const wrapper = mount(ArtistSection, {
      props: {
        artist: snapshot.artist,
        assets: snapshot.assets,
        locale: 'zh-CN',
      },
    })

    expect(wrapper.get('h2').text()).toBe('王子健')
    expect(wrapper.find(`#${snapshot.artist.id}`).exists()).toBe(true)
    expect(wrapper.text()).toContain('正式资料将在授权后更新')
    expect(wrapper.get('img').attributes('loading')).toBe('lazy')
    expect(wrapper.find('[aria-label="微博"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="哔哩哔哩"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="抖音"]').exists()).toBe(true)
  })
})
