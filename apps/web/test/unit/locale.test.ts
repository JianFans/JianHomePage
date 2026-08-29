import { detectLocale } from '../../composables/useLocale'

describe('detectLocale', () => {
  it('优先使用已保存的支持语言且不提示', () => {
    expect(detectLocale({ stored: 'en', browser: ['zh-CN'] })).toEqual({
      locale: 'en',
      notify: false,
    })
  })

  it('根据浏览器语言自动选择英文并提示', () => {
    expect(detectLocale({ stored: null, browser: ['fr-FR', 'en-US'] })).toEqual({
      locale: 'en',
      notify: true,
    })
  })

  it('识别浏览器的简体中文变体', () => {
    expect(detectLocale({ stored: null, browser: ['zh-Hans-SG'] })).toEqual({
      locale: 'zh-CN',
      notify: false,
    })
  })

  it('忽略无效偏好并在无法识别时回退简体中文', () => {
    expect(detectLocale({ stored: 'fr', browser: [] })).toEqual({
      locale: 'zh-CN',
      notify: false,
    })
  })

  it('持久化偏好只接受规范语言代码', () => {
    expect(detectLocale({ stored: 'en-US', browser: ['zh-CN'] })).toEqual({
      locale: 'zh-CN',
      notify: false,
    })
  })
})
