import type { YujianContentSnapshot } from '@yujian/schema'
import fixture from '../../../../content/fixtures/homepage.json'
import {
  buildStructuredData,
  serializeStructuredData,
} from '../../utils/structured-data'

const snapshot = fixture as unknown as YujianContentSnapshot

describe('首页结构化数据', () => {
  it('输出音乐人、曲目、影像和现场图谱', () => {
    const data = buildStructuredData(snapshot, 'zh-CN')
    const types = data['@graph'].map(item => item['@type'])

    expect(data['@context']).toBe('https://schema.org')
    expect(types).toEqual(expect.arrayContaining([
      'MusicGroup',
      'MusicRecording',
      'VideoObject',
      'MusicEvent',
    ]))
    expect(data['@graph'].find(item => item['@type'] === 'MusicGroup')).toMatchObject({
      name: '王子健',
      url: 'https://yujian.me',
    })
    expect(data['@graph'].find(item => item['@type'] === 'MusicRecording')).toMatchObject({
      duration: 'PT180S',
      name: '示例曲目 01',
    })
  })

  it('序列化时转义可提前结束脚本标签的内容', () => {
    const unsafe = structuredClone(snapshot)
    unsafe.site.artistName['zh-CN'] = '</script><img src=x onerror=alert(1)>'

    const serialized = serializeStructuredData(buildStructuredData(unsafe, 'zh-CN'))

    expect(serialized).not.toContain('</script>')
    expect(serialized).toContain('\\u003C/script\\u003E')
  })
})
