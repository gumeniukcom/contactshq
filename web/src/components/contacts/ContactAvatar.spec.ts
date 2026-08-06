import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import ContactAvatar from './ContactAvatar.vue'

describe('ContactAvatar', () => {
  it('renders initials from first and last name', () => {
    const wrapper = mount(ContactAvatar, {
      props: { firstName: 'Ada', lastName: 'Lovelace' },
    })
    expect(wrapper.text()).toBe('AL')
    expect(wrapper.find('img').exists()).toBe(false)
  })

  it('falls back to "?" when there is no name', () => {
    const wrapper = mount(ContactAvatar, { props: {} })
    expect(wrapper.text()).toBe('?')
  })

  // Raw base64 without a data: prefix is what the vCard PHOTO property carries; serving it
  // straight into src produced 431s, so the magic bytes decide the media type.
  it('prefixes raw JPEG base64 with a data URI', () => {
    const wrapper = mount(ContactAvatar, {
      props: { firstName: 'Ada', photoUri: '/9j/abc' },
    })
    expect(wrapper.find('img').attributes('src')).toBe('data:image/jpeg;base64,/9j/abc')
  })

  it('prefixes raw PNG base64 with a data URI', () => {
    const wrapper = mount(ContactAvatar, {
      props: { firstName: 'Ada', photoUri: 'iVBORabc' },
    })
    expect(wrapper.find('img').attributes('src')).toBe('data:image/png;base64,iVBORabc')
  })

  it('passes through an already-usable URL or data URI', () => {
    const wrapper = mount(ContactAvatar, {
      props: { photoUri: 'https://example.com/a.png' },
    })
    expect(wrapper.find('img').attributes('src')).toBe('https://example.com/a.png')
  })

  it('ignores a photo in an unrecognised format rather than requesting it', () => {
    const wrapper = mount(ContactAvatar, {
      props: { firstName: 'Ada', lastName: 'Lovelace', photoUri: 'not-base64-anything' },
    })
    expect(wrapper.find('img').exists()).toBe(false)
    expect(wrapper.text()).toBe('AL')
  })

  // Guards the bug this file was written for: ContactViewView passed :contact="contact",
  // which lands in none of the declared props and renders "?" on every card.
  it('renders "?" when handed a contact object instead of fields', () => {
    const wrapper = mount(ContactAvatar, {
      // Deliberately the wrong shape — this is the regression being guarded.
      props: { contact: { first_name: 'Ada', last_name: 'Lovelace' } },
    })
    expect(wrapper.text()).toBe('?')
  })
})
