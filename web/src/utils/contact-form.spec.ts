import { describe, it, expect } from 'vitest'
import { toFieldsPayload, formFromContact, emptyForm } from './contact-form'
import type { Contact } from '@/types'

describe('toFieldsPayload', () => {
  it('drops blank rows so an untouched row does not become an empty vCard property', () => {
    const form = emptyForm()
    form.emails = [
      { value: '', type: '' },
      { value: 'a@example.com', type: 'work' },
    ]
    form.phones = [{ value: '  ', type: '' }]
    form.categories = ['', 'vip']

    const payload = toFieldsPayload(form)

    expect(payload.emails).toEqual([{ value: 'a@example.com', type: 'work' }])
    expect(payload.phones).toEqual([])
    expect(payload.categories).toEqual(['vip'])
  })

  it('trims values', () => {
    const form = emptyForm()
    form.emails = [{ value: '  a@example.com  ', type: '' }]

    expect(toFieldsPayload(form).emails[0].value).toBe('a@example.com')
  })

  it('converts dates from the HTML input format to the vCard format', () => {
    const form = emptyForm()
    form.bday = '1990-01-31'
    form.anniversary = '2020-12-01'

    const payload = toFieldsPayload(form)

    expect(payload.bday).toBe('19900131')
    expect(payload.anniversary).toBe('20201201')
  })

  it('leaves empty dates empty rather than sending a bare separator', () => {
    expect(toFieldsPayload(emptyForm()).bday).toBe('')
  })

  it('never sends a vcard_data field — the server owns vCard construction', () => {
    const payload = toFieldsPayload(emptyForm()) as unknown as Record<string, unknown>
    expect(payload.vcard_data).toBeUndefined()
  })
})

describe('formFromContact', () => {
  it('prefers child rows over the denormalised primary fields', () => {
    const contact: Partial<Contact> = {
      first_name: 'Jane',
      email: 'primary@example.com',
      emails: [
        { id: '1', contact_id: 'c', value: 'a@example.com', type: 'work', pref: 1, label: '' },
        { id: '2', contact_id: 'c', value: 'b@example.com', type: 'home', pref: 0, label: '' },
      ],
    }

    const form = formFromContact(contact)

    expect(form.emails).toEqual([
      { value: 'a@example.com', type: 'work' },
      { value: 'b@example.com', type: 'home' },
    ])
  })

  it('falls back to the primary field when child rows are absent', () => {
    const form = formFromContact({ email: 'primary@example.com', phone: '+15551234567' })

    expect(form.emails).toEqual([{ value: 'primary@example.com', type: '' }])
    expect(form.phones).toEqual([{ value: '+15551234567', type: '' }])
  })

  it('offers one blank row when the contact has neither', () => {
    const form = formFromContact({})

    expect(form.emails).toEqual([{ value: '', type: '' }])
  })

  it('converts a vCard birthday to the HTML date input format', () => {
    expect(formFromContact({ bday: '19900131' }).bday).toBe('1990-01-31')
    expect(formFromContact({ bday: '1990-01-31' }).bday).toBe('1990-01-31')
    expect(formFromContact({ bday: '' }).bday).toBe('')
  })

  it('round-trips a birthday through the form and back', () => {
    const form = formFromContact({ bday: '19900131' })
    expect(toFieldsPayload(form).bday).toBe('19900131')
  })
})

// `geo` has no input in ContactForm.vue: it is carried through the form untouched so that an
// edit does not clear it. `fields` is a full replacement of the managed set server-side, so a
// payload that omits geo deletes the stored GEO. These assertions are the only guard — there
// is no visible control whose disappearance would be noticed.
describe('geo round-trip (no visible input)', () => {
  it('reads geo off the contact', () => {
    expect(formFromContact({ geo: 'geo:52.5,13.4' }).geo).toBe('geo:52.5,13.4')
  })

  it('sends geo back unchanged', () => {
    const form = formFromContact({ geo: 'geo:52.5,13.4' })
    expect(toFieldsPayload(form).geo).toBe('geo:52.5,13.4')
  })

  it('starts empty on a new contact', () => {
    expect(emptyForm().geo).toBe('')
    expect(toFieldsPayload(emptyForm()).geo).toBe('')
  })
})
