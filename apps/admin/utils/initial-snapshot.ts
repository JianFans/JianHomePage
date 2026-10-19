import type { PlatformLink, YujianContentSnapshot } from '@yujian/schema'
import { createEntityId } from './identity'

/** 创建可保存的正式起始结构，不携带开发素材或虚构的音乐、视频与现场。 */
export function createInitialSnapshot(): YujianContentSnapshot {
  const artistId = createEntityId('artist_')
  const platformLinks: PlatformLink[] = [
    { provider: 'netease-music', url: 'https://music.163.com/#/artist?id=12382128' },
    { provider: 'qq-music', url: 'https://y.qq.com/n/ryqq_v2/singer/0036zydh4H05PB' },
    { provider: 'weibo', url: 'https://weibo.com/u/5693099362' },
    { provider: 'bilibili', url: 'https://space.bilibili.com/21096618' },
    { provider: 'douyin', url: 'https://www.douyin.com/user/MS4wLjABAAAABfp-B2z7duVeezC81eAOicdyJ9pn_ANxkbCWVT__uR0' },
  ]
  return {
    schemaVersion: '1.0.0',
    releaseId: createEntityId('rel_'),
    generatedAt: new Date().toISOString(),
    site: {
      brand: { 'zh-CN': '遇健我', en: 'Meet Jian' },
      artistName: { 'zh-CN': '王子健', en: 'Wang Zijian' },
      defaultLocale: 'zh-CN', supportedLocales: ['zh-CN', 'en'], canonicalUrl: 'https://yujian.me',
      socialLinks: structuredClone(platformLinks),
      seo: {
        title: { 'zh-CN': '遇健我 · 王子健', en: 'Meet Jian · Wang Zijian' },
        description: { 'zh-CN': '音乐人王子健的粉丝站', en: 'A fan site for musician Wang Zijian' },
      },
      futureNavigation: [],
    },
    homepage: { sections: [
      { id: createEntityId('section_'), type: 'hero', enabled: true, layoutVariant: 'immersive', itemIds: [] },
      { id: createEntityId('section_'), type: 'music', enabled: true, layoutVariant: 'cover-reel', itemIds: [], limit: 5 },
      { id: createEntityId('section_'), type: 'video', enabled: true, layoutVariant: 'asymmetric', itemIds: [], limit: 3 },
      { id: createEntityId('section_'), type: 'event', enabled: true, layoutVariant: 'timeline', itemIds: [], limit: 2 },
      { id: createEntityId('section_'), type: 'moment', enabled: true, layoutVariant: 'mosaic', itemIds: [], limit: 3 },
      { id: createEntityId('section_'), type: 'artist', enabled: true, layoutVariant: 'split', itemIds: [artistId] },
    ] },
    heroSlides: [], releases: [], tracks: [], videos: [], events: [], moments: [], assets: [],
    artist: {
      id: artistId, name: { 'zh-CN': '王子健', en: 'Wang Zijian' },
      shortBio: { 'zh-CN': '音乐人', en: 'Musician' }, platformLinks,
    },
  }
}
