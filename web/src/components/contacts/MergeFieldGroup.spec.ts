import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import MergeFieldGroup from './MergeFieldGroup.vue'
import type { MergeFieldGroup as Group } from '@/utils/merge'

const emailGroup: Group = {
  spec: { property: 'EMAIL', label: 'Emails', arity: 'MULTI' },
  differs: true,
  candidates: [
    { id: 'a1', property: 'EMAIL', value: 'work@example.com', params: { TYPE: 'work' }, side: 'winner' },
    { id: 'a2', property: 'EMAIL', value: 'alt@example.com', side: 'winner' },
    { id: 'b1', property: 'EMAIL', value: 'home@example.com', side: 'loser' },
  ],
}

const nameGroup: Group = {
  spec: { property: 'FN', label: 'Display name', arity: 'SINGLETON' },
  differs: false,
  candidates: [{ id: 'n1', property: 'FN', value: 'Ada Lovelace', side: 'both' }],
}

describe('MergeFieldGroup', () => {
  // The acceptance criterion for 5.2: two emails on one side and one on the other must render
  // three independent choices, not a single "take A or take B".
  it('renders one checkbox per value of a multi-valued property', () => {
    const wrapper = mount(MergeFieldGroup, {
      props: { group: emailGroup, selected: ['a1', 'a2', 'b1'] },
    })

    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(3)
    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(0)
  })

  it('renders radios for a single-valued property', () => {
    const wrapper = mount(MergeFieldGroup, {
      props: { group: nameGroup, selected: ['n1'] },
    })

    expect(wrapper.findAll('input[type="radio"]')).toHaveLength(1)
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(0)
  })

  it('reflects the current selection', () => {
    const wrapper = mount(MergeFieldGroup, {
      props: { group: emailGroup, selected: ['a1'] },
    })

    const checked = wrapper
      .findAll('input[type="checkbox"]')
      .map((input) => (input.element as HTMLInputElement).checked)
    expect(checked).toEqual([true, false, false])
  })

  it('emits the new set when a value is unticked', async () => {
    const wrapper = mount(MergeFieldGroup, {
      props: { group: emailGroup, selected: ['a1', 'a2', 'b1'] },
    })

    await wrapper.findAll('input[type="checkbox"]')[1].setValue(false)

    expect(wrapper.emitted('update')?.[0]).toEqual([['a1', 'b1']])
  })

  it('emits the new set when a value is ticked', async () => {
    const wrapper = mount(MergeFieldGroup, {
      props: { group: emailGroup, selected: ['a1'] },
    })

    await wrapper.findAll('input[type="checkbox"]')[2].setValue(true)

    expect(wrapper.emitted('update')?.[0]).toEqual([['a1', 'b1']])
  })

  it('emits a single id for a radio choice', async () => {
    const group: Group = {
      ...nameGroup,
      differs: true,
      candidates: [
        { id: 'n1', property: 'FN', value: 'Ada Lovelace', side: 'winner' },
        { id: 'n2', property: 'FN', value: 'Ada L', side: 'loser' },
      ],
    }
    const wrapper = mount(MergeFieldGroup, { props: { group, selected: ['n1'] } })

    await wrapper.findAll('input[type="radio"]')[1].setValue(true)

    expect(wrapper.emitted('update')?.[0]).toEqual([['n2']])
  })

  // Colour alone was the only signal before, which is no signal at all for some readers.
  it('marks a differing group in words', () => {
    const wrapper = mount(MergeFieldGroup, { props: { group: emailGroup, selected: [] } })
    expect(wrapper.text()).toContain('differs')
  })

  it('says so when the sides agree', () => {
    const wrapper = mount(MergeFieldGroup, { props: { group: nameGroup, selected: ['n1'] } })
    expect(wrapper.text()).toContain('identical')
    expect(wrapper.text()).not.toContain('differs')
  })

  it('labels a value present on both records', () => {
    const wrapper = mount(MergeFieldGroup, { props: { group: nameGroup, selected: ['n1'] } })
    expect(wrapper.text()).toContain('In both')
  })

  it('shows the parameter that distinguishes two values of the same property', () => {
    const wrapper = mount(MergeFieldGroup, { props: { group: emailGroup, selected: [] } })
    expect(wrapper.text()).toContain('work')
  })
})
