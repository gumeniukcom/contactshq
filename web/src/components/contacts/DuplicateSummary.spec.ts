import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import DuplicateSummary from './DuplicateSummary.vue'
import type { Contact } from '@/types'

function contact(overrides: Partial<Contact> = {}): Contact {
  return {
    id: 'c1',
    address_book_id: 'ab1',
    uid: 'c1',
    first_name: 'Ada',
    last_name: 'Lovelace',
    email: 'ada@example.com',
    phone: '+15550001',
    created_at: '2026-01-01T00:00:00Z',
    ...overrides,
  } as Contact
}

describe('DuplicateSummary', () => {
  it('shows the name, email and phone', () => {
    const wrapper = mount(DuplicateSummary, { props: { contact: contact() } })

    expect(wrapper.text()).toContain('Ada Lovelace')
    expect(wrapper.text()).toContain('ada@example.com')
    expect(wrapper.text()).toContain('+15550001')
  })

  it('falls back to the email when a contact has no name', () => {
    const wrapper = mount(DuplicateSummary, {
      props: { contact: contact({ first_name: '', last_name: '' }) },
    })
    expect(wrapper.text()).toContain('ada@example.com')
  })

  it('says so when there is nothing to show', () => {
    const wrapper = mount(DuplicateSummary, {
      props: { contact: contact({ first_name: '', last_name: '', email: '', phone: '' }) },
    })
    expect(wrapper.text()).toContain('(no name)')
  })

  // The list only has the denormalised columns; the badge must not appear there and claim
  // values that were never loaded.
  it('shows no badge when the child collections were not loaded', () => {
    const wrapper = mount(DuplicateSummary, { props: { contact: contact() } })

    // Asserted on the element, not the text: the phone number itself starts with "+1".
    expect(wrapper.find('.bg-accent\\/10').exists()).toBe(false)
    expect(wrapper.text()).toContain('+15550001')
  })

  it('counts the extra values when the collections are present', () => {
    const wrapper = mount(DuplicateSummary, {
      props: {
        contact: contact({
          emails: [
            { id: 'e1', contact_id: 'c1', value: 'ada@example.com', type: '', pref: 0, label: '' },
            { id: 'e2', contact_id: 'c1', value: 'work@example.com', type: '', pref: 0, label: '' },
            { id: 'e3', contact_id: 'c1', value: 'alt@example.com', type: '', pref: 0, label: '' },
          ],
        } as Partial<Contact>),
      },
    })

    expect(wrapper.text()).toContain('ada@example.com')
    expect(wrapper.text()).toContain('+2')
  })
})
