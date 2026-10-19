import { mount } from '@vue/test-utils'
import { expect, it } from 'vitest'
import StudioVersionBar from '../../components/StudioVersionBar.vue'
import type { AdminVersion } from '../../utils/admin-api'

it.each([0, 2, 20])('retains the selected historical version after refreshing to %i recent versions', async (count) => {
  const current: AdminVersion = { id: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', name: 'Selected history', status: 'archived', revision: 1, snapshot: {}, checksum: 'sha256:history' }
  const recent = Array.from({ length: count }, (_, index) => ({ ...current, id: `ver_0f768b7b-3272-4a05-a829-e77b743f9996${index}`, name: `Recent ${index}` }))
  const wrapper = mount(StudioVersionBar, { props: { version: current, versions: [...recent, current], locale: 'en', busy: false, canClone: true } })
  try {
    await wrapper.setProps({ versions: recent })
    const select = wrapper.get('select')
    expect(select.element.value).toBe(current.id)
    expect(select.attributes('disabled')).toBeUndefined()
    expect(wrapper.findAll('option')).toHaveLength(count + 1)
    expect(wrapper.get(`[data-testid="version-${current.id}"]`).attributes('aria-pressed')).toBe('true')
    if (count > 0) {
      await select.setValue(recent[0]!.id)
      expect(wrapper.emitted('select')).toEqual([[recent[0]!.id]])
      expect(select.element.value).toBe(current.id)
    }
    await wrapper.setProps({ versions: [...recent, { ...current, name: 'Refreshed history' }] })
    expect(wrapper.findAll('option')).toHaveLength(count + 1)
    expect(select.element.value).toBe(current.id)
    expect(wrapper.get(`option[value="${current.id}"]`).text()).toContain('Refreshed history')
    expect(recent).toHaveLength(count)
  } finally { wrapper.unmount() }
})

it('keeps the confirmed version selected until the parent accepts a change', async () => {
  const current: AdminVersion = { id: 'ver_14cea400-a42f-4560-9770-3de88cae9d5a', name: 'Current', status: 'draft', revision: 1, snapshot: {}, checksum: 'sha256:current' }
  const history: AdminVersion = { ...current, id: 'ver_8bb3a437-061d-4dcd-805e-7d5e6f716921', name: 'History', status: 'archived' }
  const wrapper = mount(StudioVersionBar, { props: { version: current, versions: [current, history], locale: 'en', busy: false, canClone: true } })
  try {
    const select = wrapper.get('select')
    await select.setValue('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
    expect(wrapper.emitted('select')).toEqual([['ver_8bb3a437-061d-4dcd-805e-7d5e6f716921']])
    expect(select.element.value).toBe('ver_14cea400-a42f-4560-9770-3de88cae9d5a')
    await select.setValue('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
    expect(wrapper.emitted('select')).toEqual([['ver_8bb3a437-061d-4dcd-805e-7d5e6f716921'], ['ver_8bb3a437-061d-4dcd-805e-7d5e6f716921']])
    expect(select.element.value).toBe('ver_14cea400-a42f-4560-9770-3de88cae9d5a')
    await wrapper.setProps({ version: history })
    expect(select.element.value).toBe('ver_8bb3a437-061d-4dcd-805e-7d5e6f716921')
  } finally { wrapper.unmount() }
})
