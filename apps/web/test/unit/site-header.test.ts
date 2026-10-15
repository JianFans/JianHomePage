import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import SiteHeader from '../../components/site/SiteHeader.vue'

afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

describe('Header scroll control', () => {
  it('超过阈值才显示回顶，减少动态效果时即时滚动且卸载清理', async () => {
    vi.stubGlobal('matchMedia', vi.fn().mockReturnValue({ matches: true }))
    const scroll = vi.spyOn(window, 'scrollTo').mockImplementation(() => {})
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(0)
    const remove = vi.spyOn(window, 'removeEventListener')
    const wrapper = mount(SiteHeader, { props: { brand: '遇健我', artistName: '王子健', locale: 'en' } })
    expect(wrapper.find('[data-testid="back-to-top"]').exists()).toBe(false)
    expect(wrapper.find('.site-header__top-slot').exists()).toBe(true)
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(481)
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    await wrapper.get('[data-testid="back-to-top"]').trigger('click')
    expect(scroll).toHaveBeenCalledWith({ top: 0, behavior: 'instant' })
    vi.spyOn(window, 'scrollY', 'get').mockReturnValue(0)
    window.dispatchEvent(new Event('scroll'))
    await nextTick()
    expect(wrapper.find('[data-testid="back-to-top"]').exists()).toBe(false)
    expect(wrapper.find('[aria-label="切换为中文"]').exists()).toBe(true)
    wrapper.unmount()
    expect(remove).toHaveBeenCalledWith('scroll', expect.any(Function))
  })
})
