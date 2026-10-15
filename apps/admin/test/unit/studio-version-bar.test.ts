import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import StudioVersionBar from '../../components/StudioVersionBar.vue'
import type { AdminVersion } from '../../utils/admin-api'

it('keeps the confirmed version selected until the parent accepts a change', async () => {
  const current: AdminVersion = { id: 'ver_current', name: 'Current', status: 'draft', revision: 1, snapshot: {}, checksum: 'sha256:current' }
  const history: AdminVersion = { ...current, id: 'ver_history', name: 'History', status: 'archived' }
  const wrapper = mount(StudioVersionBar, { props: { version: current, versions: [current, history], locale: 'en', busy: false, canClone: true } })
  try {
    const select = wrapper.get('select')
    await select.setValue('ver_history')
    expect(wrapper.emitted('select')).toEqual([['ver_history']])
    expect(select.element.value).toBe('ver_current')
    await select.setValue('ver_history')
    expect(wrapper.emitted('select')).toEqual([['ver_history'], ['ver_history']])
    expect(select.element.value).toBe('ver_current')
    await wrapper.setProps({ version: history })
    expect(select.element.value).toBe('ver_history')
  } finally { wrapper.unmount() }
})
