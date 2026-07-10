import type { Contact, ContactFieldsPayload, ContactFormData } from '@/types'

/**
 * Convert a vCard date string (YYYYMMDD or T-format) to the HTML date input format.
 */
function toInputDate(vdate: string): string {
  if (!vdate) return ''
  if (/^\d{8}$/.test(vdate)) {
    return `${vdate.slice(0, 4)}-${vdate.slice(4, 6)}-${vdate.slice(6, 8)}`
  }
  return vdate.slice(0, 10) // already ISO-like
}

/** Convert an HTML date input value (YYYY-MM-DD) to vCard date format (YYYYMMDD). */
function toVCardDate(d: string): string {
  return d ? d.replace(/-/g, '') : ''
}

/**
 * Build the structured payload the API merges into the stored vCard.
 *
 * The form used to assemble a whole vCard here and post it as a replacement. That threw
 * away every property the form does not model — the photo and custom fields of any
 * contact synced from Google or a phone — because a rebuilt card can only contain what
 * the form knows about. The server owns vCard construction now.
 */
export function toFieldsPayload(form: ContactFormData): ContactFieldsPayload {
  const values = (rows: { value: string; type?: string }[]) =>
    rows.filter((r) => r.value.trim() !== '').map((r) => ({ value: r.value.trim(), type: r.type || '' }))

  return {
    first_name: form.first_name,
    last_name: form.last_name,
    middle_name: form.middle_name,
    name_prefix: form.name_prefix,
    name_suffix: form.name_suffix,
    nickname: form.nickname,
    org: form.org,
    department: form.department,
    title: form.title,
    role: form.role,
    note: form.note,
    gender: form.gender,
    tz: form.tz,
    bday: toVCardDate(form.bday),
    anniversary: toVCardDate(form.anniversary),
    emails: values(form.emails),
    phones: values(form.phones),
    urls: values(form.urls),
    ims: values(form.ims),
    addresses: form.addresses.map((a) => ({
      type: a.type || '',
      street: a.street || '',
      city: a.city || '',
      region: a.region || '',
      postal_code: a.postal_code || '',
      country: a.country || '',
    })),
    categories: form.categories.filter((c) => c.trim() !== ''),
  }
}

/**
 * Convert an API Contact object into the flat ContactFormData used by ContactForm.vue.
 * If relations (emails/phones/...) are absent, falls back to the denormalised primary fields.
 */
export function formFromContact(contact: Partial<Contact>): ContactFormData {
  return {
    uid: contact.uid,
    first_name: contact.first_name || '',
    last_name: contact.last_name || '',
    middle_name: contact.middle_name || '',
    name_prefix: contact.name_prefix || '',
    name_suffix: contact.name_suffix || '',
    nickname: contact.nickname || '',
    org: contact.org || '',
    department: contact.department || '',
    title: contact.title || '',
    role: contact.role || '',
    note: contact.note || '',
    bday: toInputDate(contact.bday || ''),
    anniversary: toInputDate(contact.anniversary || ''),
    gender: contact.gender || '',
    tz: contact.tz || '',
    emails:
      contact.emails?.map((e) => ({ value: e.value, type: e.type || '' })) ??
      (contact.email ? [{ value: contact.email, type: '' }] : [{ value: '', type: '' }]),
    phones:
      contact.phones?.map((p) => ({ value: p.value, type: p.type || '' })) ??
      (contact.phone ? [{ value: contact.phone, type: '' }] : [{ value: '', type: '' }]),
    urls: contact.urls?.map((u) => ({ value: u.value, type: u.type || '' })) ?? [],
    ims: contact.ims?.map((im) => ({ value: im.value, type: im.type || '' })) ?? [],
    addresses:
      contact.addresses?.map((a) => ({
        street: a.street || '',
        city: a.city || '',
        region: a.region || '',
        postal_code: a.postal_code || '',
        country: a.country || '',
        type: a.type || '',
      })) ?? [],
    categories: contact.categories?.map((c) => c.value) ?? [],
  }
}

/** Return a fresh empty ContactFormData with one blank email and phone row. */
export function emptyForm(): ContactFormData {
  return {
    first_name: '',
    last_name: '',
    middle_name: '',
    name_prefix: '',
    name_suffix: '',
    nickname: '',
    org: '',
    department: '',
    title: '',
    role: '',
    note: '',
    bday: '',
    anniversary: '',
    gender: '',
    tz: '',
    emails: [{ value: '', type: '' }],
    phones: [{ value: '', type: '' }],
    urls: [],
    ims: [],
    addresses: [],
    categories: [],
  }
}
