import { describe, it, expect } from 'vitest'
import {
  buildMergeModel,
  buildMergePayload,
  defaultSelection,
  discardedBySelection,
  mergeDiffers,
  previewFromSelection,
} from './merge'
import type { ValueCandidate } from '@/types'

const candidates: ValueCandidate[] = [
  { id: 'fn-a', property: 'FN', value: 'Ada Lovelace', side: 'winner' },
  { id: 'fn-b', property: 'FN', value: 'Ada L', side: 'loser' },
  {
    id: 'em-work',
    property: 'EMAIL',
    value: 'ada.work@example.com',
    params: { TYPE: 'work' },
    side: 'winner',
  },
  {
    id: 'em-home',
    property: 'EMAIL',
    value: 'ada.home@example.com',
    params: { TYPE: 'home' },
    side: 'loser',
  },
  { id: 'tel-shared', property: 'TEL', value: '+15550001', side: 'both' },
  { id: 'note-b', property: 'NOTE', value: 'met at a conference', side: 'loser' },
  { id: 'x-thing', property: 'X-CUSTOM', value: 'whatever', side: 'winner' },
]

describe('buildMergeModel', () => {
  it('groups candidates by property in display order', () => {
    const model = buildMergeModel(candidates)
    expect(model.groups.map((g) => g.spec.property)).toEqual(['FN', 'NOTE', 'EMAIL', 'TEL'])
  })

  it('marks a group as differing only when the sides disagree', () => {
    const model = buildMergeModel(candidates)
    const byProperty = Object.fromEntries(model.groups.map((g) => [g.spec.property, g]))

    expect(byProperty.EMAIL.differs).toBe(true)
    expect(byProperty.TEL.differs).toBe(false)
  })

  it('keeps properties it does not render rather than dropping them', () => {
    const model = buildMergeModel(candidates)
    expect(model.unlisted.map((c) => c.property)).toEqual(['X-CUSTOM'])
  })
})

describe('defaultSelection', () => {
  it('keeps every value of a multi-valued property', () => {
    const model = buildMergeModel(candidates)
    const selection = defaultSelection(model, 'a')

    expect(selection.EMAIL.sort()).toEqual(['em-home', 'em-work'])
  })

  it("takes the winner's value for a single-valued property", () => {
    const model = buildMergeModel(candidates)

    expect(defaultSelection(model, 'a').FN).toEqual(['fn-a'])
    expect(defaultSelection(model, 'b').FN).toEqual(['fn-b'])
  })

  // An empty name on the winning record is not a choice anyone means to make.
  it('falls back to the other side when the winner has no value', () => {
    const model = buildMergeModel(candidates)
    expect(defaultSelection(model, 'a').NOTE).toEqual(['note-b'])
  })
})

describe('previewFromSelection', () => {
  it('shows only what survives', () => {
    const model = buildMergeModel(candidates)
    const selection = { ...defaultSelection(model, 'a'), EMAIL: ['em-work'] }

    const preview = previewFromSelection(model, selection)
    const emails = preview.find((g) => g.spec.property === 'EMAIL')
    expect(emails?.candidates.map((c) => c.value)).toEqual(['ada.work@example.com'])
  })

  it('omits a property with nothing selected', () => {
    const model = buildMergeModel(candidates)
    const selection = { ...defaultSelection(model, 'a'), NOTE: [] }

    expect(previewFromSelection(model, selection).map((g) => g.spec.property)).not.toContain('NOTE')
  })
})

describe('discardedBySelection', () => {
  it('names exactly what will be lost', () => {
    const model = buildMergeModel(candidates)
    const selection = { ...defaultSelection(model, 'a'), EMAIL: ['em-work'] }

    expect(discardedBySelection(model, selection).map((c) => c.value)).toEqual([
      'Ada L',
      'ada.home@example.com',
    ])
  })

  it('is empty when nothing is dropped', () => {
    const model = buildMergeModel(candidates)
    const selection = {
      FN: ['fn-a', 'fn-b'],
      NOTE: ['note-b'],
      EMAIL: ['em-work', 'em-home'],
      TEL: ['tel-shared'],
    }
    expect(discardedBySelection(model, selection)).toEqual([])
  })
})

describe('buildMergePayload', () => {
  // The regression this whole module exists for: the old screen sent snake_case contact
  // fields, which the server — resolving by vCard property name — never matched.
  it('keys the selection by vCard property names, not contact fields', () => {
    const model = buildMergeModel(candidates)
    const payload = buildMergePayload({
      dupId: 'd1',
      contactAId: 'a',
      contactBId: 'b',
      winner: 'a',
      selection: defaultSelection(model, 'a'),
    })

    expect(Object.keys(payload.selection ?? {})).toContain('EMAIL')
    expect(Object.keys(payload.selection ?? {})).toContain('TEL')
    expect(Object.keys(payload.selection ?? {})).not.toContain('email')
    expect(Object.keys(payload.selection ?? {})).not.toContain('phone')
  })

  // The old view decided the winner by counting which side more fields came from, so taking
  // the other record's phone number could silently change which UID survived.
  it('takes the winner from the explicit choice, not from the selection', () => {
    const model = buildMergeModel(candidates)
    // Almost everything selected belongs to B, yet A was chosen to survive.
    const selection = { ...defaultSelection(model, 'a'), FN: ['fn-b'], NOTE: ['note-b'] }

    const payload = buildMergePayload({
      dupId: 'd1',
      contactAId: 'a',
      contactBId: 'b',
      winner: 'a',
      selection,
    })

    expect(payload.winner_id).toBe('a')
    expect(payload.loser_id).toBe('b')
  })

  it('carries the pair id so the merge can be recorded against it', () => {
    const payload = buildMergePayload({
      dupId: 'd1',
      contactAId: 'a',
      contactBId: 'b',
      winner: 'b',
      selection: {},
    })

    expect(payload.dup_id).toBe('d1')
    expect(payload.winner_id).toBe('b')
    expect(payload.loser_id).toBe('a')
  })
})

describe('mergeDiffers', () => {
  it('is false for the untouched default', () => {
    const model = buildMergeModel(candidates)
    expect(mergeDiffers(model, defaultSelection(model, 'a'), 'a')).toBe(false)
  })

  it('is true once a value is deselected', () => {
    const model = buildMergeModel(candidates)
    const selection = { ...defaultSelection(model, 'a'), EMAIL: ['em-work'] }
    expect(mergeDiffers(model, selection, 'a')).toBe(true)
  })
})
