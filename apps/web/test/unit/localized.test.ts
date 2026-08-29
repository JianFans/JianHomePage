import { resolveLocalized } from '../../utils/localized'

describe('resolveLocalized', () => {
  it('返回请求语言', () => {
    expect(resolveLocalized({ 'zh-CN': '作品', en: 'Release' }, 'en')).toBe('Release')
  })

  it('英文缺失时回退简体中文', () => {
    expect(resolveLocalized({ 'zh-CN': '作品' }, 'en')).toBe('作品')
  })

  it('所有文本为空时返回空字符串', () => {
    expect(resolveLocalized({}, 'en')).toBe('')
  })
})
