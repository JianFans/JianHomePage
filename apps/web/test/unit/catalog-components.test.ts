import type { AudioPlayerController, AudioPlayerStatus, AudioPlayerTrack } from '../../utils/audio-player'
import type { PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import { mount } from '@vue/test-utils'
import { ref } from 'vue'
import { vi } from 'vitest'
import fixture from '../../../../content/fixtures/homepage.json'
import EventCatalog from '../../components/event/EventCatalog.vue'
import MusicCatalog from '../../components/music/MusicCatalog.vue'
import VideoCatalog from '../../components/video/VideoCatalog.vue'

const source = fixture as unknown as YujianContentSnapshot

/** Creates the minimum reactive player state that catalog cards consume. */
function catalogPlayer(trackId?: string, status: AudioPlayerStatus = 'idle') {
  return {
    current: ref(trackId ? { id: trackId } as AudioPlayerTrack : null),
    status: ref(status),
    toggle: vi.fn(),
  } as unknown as AudioPlayerController
}

describe('目录页面组件', () => {
  it('渲染按日期排序的作品、预览状态与无效关联过滤', async () => {
    const snapshot = structuredClone(source)
    const latest = snapshot.releases[0]!
    latest.coverAssetId = 'asset_missing'
    latest.trackIds.push('track_missing' as typeof latest.trackIds[number])
    snapshot.tracks[0]!.previewAssetId = snapshot.assets.find(asset => asset.kind === 'image')!.id
    const previewTrack = snapshot.tracks.find(track => track.previewAssetId === snapshot.assets.find(asset => asset.kind === 'audio')!.id)!
    const player = catalogPlayer(previewTrack.id, 'loading')
    const wrapper = mount(MusicCatalog, { props: { snapshot, locale: 'en', player } })
    const releases = wrapper.findAll('.catalog-release')

    expect(wrapper.get('h1').text()).toBe('All music')
    expect(releases.map(release => release.attributes('id'))).toEqual(
      [...snapshot.releases].sort((left, right) => right.releaseDate.localeCompare(left.releaseDate)).map(release => release.id),
    )
    expect(releases[0]!.find('.release-cover').exists()).toBe(false)
    expect(releases[0]!.findAll('.catalog-track')).toHaveLength(1)
    expect(wrapper.get(`[aria-busy="true"]`).attributes('aria-label')).toContain('Sample Track')
    expect(wrapper.findAll('.catalog-track').some(row => row.text().includes('Sample Track 02'))).toBe(true)

    const trigger = wrapper.findAll('button').find(button => button.attributes('aria-label')?.includes(previewTrack.title.en!))!
    await trigger.trigger('click')
    expect(player.toggle).toHaveBeenCalledOnce()
    expect((player.toggle as ReturnType<typeof vi.fn>).mock.calls[0]![0]).toMatchObject({ id: previewTrack.id })

    player.status.value = 'playing'
    await wrapper.vm.$nextTick()
    expect(wrapper.get(`[aria-label*="${previewTrack.title.en}"]`).attributes('aria-pressed')).toBe('true')
  })

  it('显示没有可用作品的空状态', () => {
    const snapshot = structuredClone(source)
    snapshot.releases = []

    const wrapper = mount(MusicCatalog, { props: { snapshot, locale: 'zh-CN', player: catalogPlayer() } })

    expect(wrapper.text()).toContain('暂无音乐作品')
    expect(wrapper.findAll('.catalog-release')).toHaveLength(0)
  })

  it('仅列出海报和 HTTPS 平台有效的视频，并输出完整平台菜单', () => {
    const snapshot = structuredClone(source)
    snapshot.videos[0]!.featured = false
    snapshot.videos[1]!.featured = true
    snapshot.videos[1]!.platformLinks = Array.from({ length: 5 }, (_, index) => ({
      provider: 'website' as const,
      url: `https://example.com/platform-${index + 1}`,
    })) as [PlatformLink, ...PlatformLink[]]
    snapshot.videos[2]!.platformLinks[0]!.url = 'http://example.com/insecure'

    const wrapper = mount(VideoCatalog, { props: { snapshot, locale: 'en' } })
    const cards = wrapper.findAll('.video-card')

    expect(wrapper.get('h1').text()).toBe('All videos')
    expect(cards.map(card => card.attributes('id'))).toEqual([snapshot.videos[1]!.id, snapshot.videos[0]!.id])
    expect(cards[0]!.text()).toContain('3:00')
    expect(cards[0]!.findAll('.platform-more__menu a')).toHaveLength(4)
    expect(cards[0]!.findAll('.platform-link')).toHaveLength(6)
    expect(cards[0]!.get('.video-card__open').attributes('href')).toBe('https://example.com/platform-1')
  })

  it('显示没有有效海报和平台链接的视频空状态', () => {
    const snapshot = structuredClone(source)
    snapshot.videos = snapshot.videos.map(video => ({
      ...video,
      platformLinks: [{ ...video.platformLinks[0]!, url: 'http://example.com/insecure' }],
    }))

    const wrapper = mount(VideoCatalog, { props: { snapshot, locale: 'zh-CN' } })

    expect(wrapper.text()).toContain('暂无影像作品')
    expect(wrapper.findAll('.video-card')).toHaveLength(0)
  })

  it('按事件状态显示现场列表并隐藏未启用的视频导航', () => {
    const snapshot = structuredClone(source)
    snapshot.homepage.sections.find(section => section.type === 'video')!.enabled = false
    snapshot.events[0]!.posterAssetId = snapshot.assets.find(asset => asset.kind === 'image')!.id

    const wrapper = mount(EventCatalog, { props: { snapshot, locale: 'en' } })

    expect(wrapper.get('h1').text()).toBe('Live events')
    expect(wrapper.findAll('[data-testid="event-card"]')).toHaveLength(snapshot.events.length)
    expect(wrapper.get('[data-testid="event-card"] a').classes()).toContain('event-row--with-poster')
    expect(wrapper.find('[data-testid="event-card"] .event-poster').exists()).toBe(true)
    expect(wrapper.text()).toContain('Upcoming')
    expect(wrapper.get('time').attributes('datetime')).toBeTruthy()
    expect(wrapper.find('header a[href="/video/"]').exists()).toBe(false)
  })
})
