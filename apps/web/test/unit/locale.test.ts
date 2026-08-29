import {
	detectLocale,
	readLocalePreference,
	writeLocalePreference,
} from '../../composables/useLocale'

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

  it('将其他中文变体回退为站点默认中文', () => {
    expect(detectLocale({ stored: null, browser: ['zh-TW', 'en-US'] })).toEqual({
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

describe('语言偏好存储降级', () => {
	it('读取异常时回退为空偏好且不抛错', () => {
		const storage = { getItem: () => { throw new Error('storage blocked') } }
		expect(readLocalePreference(storage)).toBeNull()
	})

	it('写入异常时保留页面交互且不抛错', () => {
		const storage = { setItem: () => { throw new Error('quota exceeded') } }
		expect(() => writeLocalePreference(storage, 'en')).not.toThrow()
	})
})
