import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import MergePreviewCard from './MergePreviewCard.vue'
import type { MergeFieldGroup } from '@/utils/merge'
import type { ValueCandidate } from '@/types'

const preview: MergeFieldGroup[] = [
  {
    spec: { property: 'EMAIL', label: 'Emails', arity: 'MULTI' },
    differs: true,
    candidates: [{ id: 'a1', property: 'EMAIL', value: 'work@example.com', side: 'winner' }],
  },
]

const discarded: ValueCandidate[] = [
  { id: 'b1', property: 'EMAIL', value: 'home@example.com', side: 'loser' },
  { id: 'b2', property: 'TEL', value: '+15550002', side: 'loser' },
]

describe('MergePreviewCard', () => {
  it('shows what will survive', () => {
    const wrapper = mount(MergePreviewCard, { props: { preview, discarded: [] } })
    expect(wrapper.text()).toContain('work@example.com')
  })

  // Counting is not enough: "3 values will be discarded" says something is going without
  // saying whether the reader minds.
  it('names each discarded value rather than only counting them', () => {
    const wrapper = mount(MergePreviewCard, { props: { preview, discarded } })

    expect(wrapper.text()).toContain('Will be discarded (2)')
    expect(wrapper.text()).toContain('home@example.com')
    expect(wrapper.text()).toContain('+15550002')
  })

  it('labels a discarded value with the field it came from', () => {
    const wrapper = mount(MergePreviewCard, { props: { preview, discarded } })
    expect(wrapper.text()).toContain('Phones')
  })

  it('says plainly when nothing is lost', () => {
    const wrapper = mount(MergePreviewCard, { props: { preview, discarded: [] } })

    expect(wrapper.text()).toContain('Nothing is lost')
    expect(wrapper.text()).not.toContain('Will be discarded')
  })

  it('handles an empty selection without pretending there is a result', () => {
    const wrapper = mount(MergePreviewCard, { props: { preview: [], discarded } })
    expect(wrapper.text()).toContain('Nothing selected')
  })
})
