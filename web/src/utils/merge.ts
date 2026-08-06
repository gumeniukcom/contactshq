import type { MergeInput, MergeSelection, ValueCandidate } from '@/types'

/**
 * The merge screen used to send keys like `first_name` and `email`. The server resolves by
 * vCard property name — FN, EMAIL, TEL — so every key missed, the default applied, and the
 * winner's value won no matter what the user clicked. The per-field choice was decorative.
 *
 * The model here is built from the value candidates the server returns with a pair. Their ids
 * are content hashes minted server-side; nothing on this side invents one.
 */

/** How many values a property may hold. */
export type Arity = 'SINGLETON' | 'MULTI'

export interface FieldSpec {
  /** vCard property name — the key the server merges by. */
  property: string
  label: string
  arity: Arity
}

/**
 * The properties the merge screen shows, in display order. Anything a card holds that is not
 * listed still merges (the server keeps the winner's values); it simply is not offered as a
 * choice, which keeps the screen about the fields people recognise.
 */
export const MERGE_FIELD_SPECS: FieldSpec[] = [
  { property: 'FN', label: 'Display name', arity: 'SINGLETON' },
  { property: 'N', label: 'Name', arity: 'SINGLETON' },
  { property: 'NICKNAME', label: 'Nickname', arity: 'SINGLETON' },
  { property: 'ORG', label: 'Organisation', arity: 'SINGLETON' },
  { property: 'TITLE', label: 'Title', arity: 'SINGLETON' },
  { property: 'ROLE', label: 'Role', arity: 'SINGLETON' },
  { property: 'BDAY', label: 'Birthday', arity: 'SINGLETON' },
  { property: 'NOTE', label: 'Note', arity: 'SINGLETON' },
  { property: 'EMAIL', label: 'Emails', arity: 'MULTI' },
  { property: 'TEL', label: 'Phones', arity: 'MULTI' },
  { property: 'ADR', label: 'Addresses', arity: 'MULTI' },
  { property: 'URL', label: 'Websites', arity: 'MULTI' },
  { property: 'IMPP', label: 'Instant messaging', arity: 'MULTI' },
  { property: 'CATEGORIES', label: 'Categories', arity: 'MULTI' },
]

export interface MergeFieldGroup {
  spec: FieldSpec
  candidates: ValueCandidate[]
  /** True when the two sides do not offer the same set of values. */
  differs: boolean
}

export interface MergeModel {
  groups: MergeFieldGroup[]
  /** Candidates for properties outside MERGE_FIELD_SPECS, carried so they are not lost. */
  unlisted: ValueCandidate[]
}

/**
 * Groups the server's candidates by property, in the order the screen renders them.
 *
 * Values identical on both cards arrive already collapsed into one candidate marked
 * `side: 'both'` — the server does that, because it is the one that knows the parameters.
 */
export function buildMergeModel(candidates: ValueCandidate[]): MergeModel {
  const byProperty = new Map<string, ValueCandidate[]>()
  for (const c of candidates) {
    const list = byProperty.get(c.property) ?? []
    list.push(c)
    byProperty.set(c.property, list)
  }

  const groups: MergeFieldGroup[] = []
  for (const spec of MERGE_FIELD_SPECS) {
    const list = byProperty.get(spec.property)
    if (!list?.length) continue
    byProperty.delete(spec.property)
    groups.push({
      spec,
      candidates: list,
      // A property differs when either side brings something the other does not.
      differs: list.some((c) => c.side !== 'both'),
    })
  }

  const unlisted: ValueCandidate[] = []
  for (const list of byProperty.values()) unlisted.push(...list)

  return { groups, unlisted }
}

/**
 * What is selected before the user touches anything.
 *
 * MULTI keeps everything: a merge that silently dropped one of two phone numbers would be
 * the worst possible default. SINGLETON keeps the winner's value, falling back to the other
 * side when the winner has none — an empty name is not a choice anyone means to make.
 */
export function defaultSelection(model: MergeModel, winner: 'a' | 'b'): MergeSelection {
  const winnerSide = winner === 'a' ? 'winner' : 'loser'
  const selection: MergeSelection = {}

  for (const group of model.groups) {
    if (group.spec.arity === 'MULTI') {
      selection[group.spec.property] = group.candidates.map((c) => c.id)
      continue
    }

    const preferred =
      group.candidates.find((c) => c.side === winnerSide || c.side === 'both') ?? group.candidates[0]
    selection[group.spec.property] = preferred ? [preferred.id] : []
  }

  return selection
}

/** The values that will survive, in the order the merged card will hold them. */
export function previewFromSelection(model: MergeModel, selection: MergeSelection): MergeFieldGroup[] {
  return model.groups
    .map((group) => ({
      ...group,
      candidates: group.candidates.filter((c) => (selection[group.spec.property] ?? []).includes(c.id)),
    }))
    .filter((group) => group.candidates.length > 0)
}

/** The values the merge will discard — what the confirmation has to be able to name. */
export function discardedBySelection(model: MergeModel, selection: MergeSelection): ValueCandidate[] {
  const discarded: ValueCandidate[] = []
  for (const group of model.groups) {
    const kept = selection[group.spec.property] ?? []
    for (const candidate of group.candidates) {
      if (!kept.includes(candidate.id)) discarded.push(candidate)
    }
  }
  return discarded
}

/** Whether the selection differs from keeping the winner untouched. */
export function mergeDiffers(model: MergeModel, selection: MergeSelection, winner: 'a' | 'b'): boolean {
  const baseline = defaultSelection(model, winner)
  const keys = new Set([...Object.keys(baseline), ...Object.keys(selection)])
  for (const key of keys) {
    const a = [...(baseline[key] ?? [])].sort()
    const b = [...(selection[key] ?? [])].sort()
    if (a.length !== b.length || a.some((id, i) => id !== b[i])) return true
  }
  return false
}

/**
 * Builds the request.
 *
 * The winner comes from what the user explicitly chose, not from counting which side more
 * fields were taken from. The old majority vote meant picking the other record's phone number
 * could silently change which contact survived — and with it the UID every phone syncs by.
 */
export function buildMergePayload(args: {
  dupId: string
  contactAId: string
  contactBId: string
  winner: 'a' | 'b'
  selection: MergeSelection
}): MergeInput {
  const winnerId = args.winner === 'a' ? args.contactAId : args.contactBId
  const loserId = args.winner === 'a' ? args.contactBId : args.contactAId

  return {
    winner_id: winnerId,
    loser_id: loserId,
    selection: args.selection,
    dup_id: args.dupId,
  }
}
