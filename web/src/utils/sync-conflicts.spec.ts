import { describe, it, expect } from 'vitest'
import { parseFieldDiffs } from './sync-conflicts'

describe('parseFieldDiffs', () => {
  // The case that blanked both screens. JSON.parse('null') returns null rather than throwing,
  // so the try/catch these views used never fired and .forEach ran on null.
  it('returns an empty array for the literal string "null"', () => {
    expect(parseFieldDiffs('null')).toEqual([])
  })

  it('returns an empty array for an object, which is not a diff list', () => {
    expect(parseFieldDiffs('{"field":"FN"}')).toEqual([])
  })

  it('returns an empty array for malformed JSON', () => {
    expect(parseFieldDiffs('[{oops')).toEqual([])
  })

  it.each([null, undefined, ''])('returns an empty array for %p', (raw) => {
    expect(parseFieldDiffs(raw)).toEqual([])
  })

  it('passes a real diff list through unchanged', () => {
    const raw = '[{"field":"FN","local":"Ada","remote":"Ada L."}]'
    expect(parseFieldDiffs(raw)).toEqual([{ field: 'FN', local: 'Ada', remote: 'Ada L.' }])
  })

  it('returns an empty array for an empty list, which is a real state', () => {
    expect(parseFieldDiffs('[]')).toEqual([])
  })
})
