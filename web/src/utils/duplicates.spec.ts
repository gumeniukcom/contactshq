import { describe, it, expect } from 'vitest'
import {
  canKeepA,
  canKeepB,
  confidenceLabel,
  parseMatchReasons,
  reasonLabel,
  reasonLabels,
} from './duplicates'

describe('parseMatchReasons', () => {
  // Rows written before task 4.5 hold a bare array of codes.
  it('reads the legacy string[] form', () => {
    expect(parseMatchReasons('["email_match","name_exact"]')).toEqual([
      { code: 'email_match' },
      { code: 'name_exact' },
    ])
  })

  it('reads the current form with the matched value', () => {
    expect(parseMatchReasons('[{"code":"email_match","value":"a@b.c"}]')).toEqual([
      { code: 'email_match', value: 'a@b.c' },
    ])
  })

  it('reads a mixture, because a database can hold both', () => {
    expect(parseMatchReasons('["phone_match",{"code":"name_exact","value":"Ada"}]')).toEqual([
      { code: 'phone_match' },
      { code: 'name_exact', value: 'Ada' },
    ])
  })

  // An unreadable reason means "no explanation", not "the page is broken".
  it.each([
    ['invalid JSON', 'not json at all'],
    ['a truncated array', '[{"code":'],
    ['an object rather than an array', '{"code":"email_match"}'],
    ['an empty string', ''],
  ])('returns nothing for %s', (_name, raw) => {
    expect(parseMatchReasons(raw)).toEqual([])
  })

  it.each([
    ['null', null],
    ['undefined', undefined],
  ])('returns nothing for %s', (_name, raw) => {
    expect(parseMatchReasons(raw)).toEqual([])
  })

  it('skips entries that carry no code', () => {
    expect(parseMatchReasons('[{"value":"a@b.c"},{"code":"phone_match"},42,null]')).toEqual([
      { code: 'phone_match' },
    ])
  })

  it('treats an empty value as absent', () => {
    expect(parseMatchReasons('[{"code":"email_match","value":""}]')).toEqual([{ code: 'email_match' }])
  })
})

describe('reasonLabel', () => {
  it('names the matched value, which is the whole point of the newer format', () => {
    expect(reasonLabel({ code: 'email_match', value: 'a@b.c' })).toBe('Same email: a@b.c')
  })

  it('degrades to the bare label on a legacy reason', () => {
    expect(reasonLabel({ code: 'email_match' })).toBe('Same email')
  })

  it('renders an unknown code readably rather than hiding it', () => {
    expect(reasonLabel({ code: 'some_future_rule' })).toBe('some future rule')
  })

  it('labels a whole row', () => {
    expect(reasonLabels('["phone_match",{"code":"name_exact","value":"Ada"}]')).toEqual([
      'Same phone',
      'Same name: Ada',
    ])
  })
})

describe('confidenceLabel', () => {
  // The score has only ever been 1.0 or 0.8, so a percentage implied precision that is not
  // there.
  it('distinguishes the only two values the detector produces', () => {
    expect(confidenceLabel(1)).toBe('Certain match')
    expect(confidenceLabel(0.8)).toBe('Likely match')
  })
})

describe('canKeepA / canKeepB', () => {
  it('allows the shortcut only when the other side adds nothing', () => {
    expect(canKeepA({ b_subset_of_a: true })).toBe(true)
    expect(canKeepA({ b_subset_of_a: false })).toBe(false)
    expect(canKeepB({ a_subset_of_b: true })).toBe(true)
  })

  // The quick button acts without showing what it discards, so "unknown" must read as unsafe.
  it('refuses when the server said nothing', () => {
    expect(canKeepA({})).toBe(false)
    expect(canKeepB({})).toBe(false)
  })
})
