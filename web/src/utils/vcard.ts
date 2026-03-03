import type { Contact, ContactFormData, ContactFormAddress } from '@/types'

// Escape special characters inside a vCard text value.
function esc(val: string): string {
  return val.replace(/\\/g, '\\\\').replace(/;/g, '\\;').replace(/,/g, '\\,').replace(/\n/g, '\\n')
}

// Convert a vCard date string (YYYYMMDD or T-format) to HTML date input format (YYYY-MM-DD).
function toInputDate(vdate: string): string {
  if (!vdate) return ''
  if (/^\d{8}$/.test(vdate)) {
    return `${vdate.slice(0, 4)}-${vdate.slice(4, 6)}-${vdate.slice(6, 8)}`
  }
  return vdate.slice(0, 10) // already ISO-like
}

// Convert an HTML date input value (YYYY-MM-DD) to vCard date format (YYYYMMDD).
function toVCardDate(d: string): string {
  if (!d) return ''
  return d.replace(/-/g, '')
}

/**
 * Build a vCard 4.0 string from the ContactFormData collected by ContactForm.vue.
 * The resulting string is sent to the backend as `vcard_data`.
 */
export function buildVCard(form: ContactFormData): string {
  const lines: string[] = ['BEGIN:VCARD', 'VERSION:4.0']

  if (form.uid) lines.push(`UID:${form.uid}`)

  // FN (formatted name) — required field
  const fnParts = [form.name_prefix, form.first_name, form.middle_name, form.last_name, form.name_suffix]
    .map((v) => v?.trim())
    .filter(Boolean)
  lines.push(`FN:${fnParts.join(' ') || 'Unknown'}`)

  // N: last;first;middle;prefix;suffix
  lines.push(
    `N:${esc(form.last_name || '')};${esc(form.first_name || '')};${esc(form.middle_name || '')};${esc(form.name_prefix || '')};${esc(form.name_suffix || '')}`,
  )

  if (form.nickname) lines.push(`NICKNAME:${esc(form.nickname)}`)

  // ORG
  if (form.org || form.department) {
    lines.push(`ORG:${esc(form.org || '')};${esc(form.department || '')}`)
  }

  if (form.title) lines.push(`TITLE:${esc(form.title)}`)
  if (form.role) lines.push(`ROLE:${esc(form.role)}`)
  if (form.note) lines.push(`NOTE:${esc(form.note)}`)

  // Personal
  if (form.bday) lines.push(`BDAY:${toVCardDate(form.bday)}`)
  if (form.anniversary) lines.push(`ANNIVERSARY:${toVCardDate(form.anniversary)}`)
  if (form.gender) lines.push(`GENDER:${form.gender}`)
  if (form.tz) lines.push(`TZ:${form.tz}`)

  // Emails
  form.emails.forEach((e, i) => {
    if (!e.value) return
    const params: string[] = []
    if (e.type) params.push(`TYPE=${e.type.toUpperCase()}`)
    if (i === 0) params.push('PREF=1')
    lines.push(`EMAIL${params.length ? ';' + params.join(';') : ''}:${e.value}`)
  })

  // Phones
  form.phones.forEach((p, i) => {
    if (!p.value) return
    const params: string[] = []
    if (p.type) params.push(`TYPE=${p.type.toUpperCase()}`)
    if (i === 0) params.push('PREF=1')
    lines.push(`TEL${params.length ? ';' + params.join(';') : ''}:${p.value}`)
  })

  // URLs
  form.urls.forEach((u) => {
    if (!u.value) return
    const paramStr = u.type ? `;TYPE=${u.type.toUpperCase()}` : ''
    lines.push(`URL${paramStr}:${u.value}`)
  })

  // IMs
  form.ims.forEach((im) => {
    if (!im.value) return
    lines.push(`IMPP:${im.value}`)
  })

  // Addresses
  form.addresses.forEach((a: ContactFormAddress, i) => {
    const parts = [
      '',
      '',
      esc(a.street || ''),
      esc(a.city || ''),
      esc(a.region || ''),
      esc(a.postal_code || ''),
      esc(a.country || ''),
    ].join(';')
    const typeParam = a.type ? `;TYPE=${a.type.toUpperCase()}` : ''
    const pref = i === 0 ? ';PREF=1' : ''
    lines.push(`ADR${typeParam}${pref}:${parts}`)
  })

  // Categories
  if (form.categories.length > 0) {
    lines.push(`CATEGORIES:${form.categories.map(esc).join(',')}`)
  }

  lines.push('END:VCARD')
  return lines.join('\r\n')
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
