import type { MatchReason, PotentialDuplicate } from '@/types'

/**
 * Reading match_reasons is deliberately forgiving.
 *
 * The column holds JSON written by two different versions of the detector: a bare array of
 * codes on older rows, and objects carrying the matched value on newer ones. It is also just
 * a TEXT column, so a truncated or hand-edited row is possible. None of that is worth an
 * error screen on a list of duplicates — an unreadable reason means "no explanation", not
 * "the page is broken".
 */
export function parseMatchReasons(raw: string | null | undefined): MatchReason[] {
  if (!raw) return []

  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return []
  }
  if (!Array.isArray(parsed)) return []

  const out: MatchReason[] = []
  for (const item of parsed) {
    if (typeof item === 'string') {
      out.push({ code: item })
      continue
    }
    if (item && typeof item === 'object' && typeof (item as MatchReason).code === 'string') {
      const reason = item as MatchReason
      out.push({
        code: reason.code,
        value: typeof reason.value === 'string' && reason.value !== '' ? reason.value : undefined,
      })
    }
  }
  return out
}

const REASON_LABELS: Record<string, string> = {
  email_match: 'Same email',
  phone_match: 'Same phone',
  name_exact: 'Same name',
  name_similar: 'Similar name',
}

/**
 * Renders one reason for a human.
 *
 * The value comes from the stored reason rather than being recomputed from the two contacts:
 * with several phone numbers on a record, guessing which one matched is wrong about as often
 * as it is right.
 */
export function reasonLabel(reason: MatchReason): string {
  const label = REASON_LABELS[reason.code] ?? reason.code.replace(/_/g, ' ')
  return reason.value ? `${label}: ${reason.value}` : label
}

export function reasonLabels(raw: string | null | undefined): string[] {
  return parseMatchReasons(raw).map(reasonLabel)
}

/**
 * The score has only ever taken two values — 1.0 for an exact email, 0.8 for a phone — so
 * rendering it as a percentage implied a precision that does not exist.
 */
export function confidenceLabel(score: number): 'Certain match' | 'Likely match' {
  return score >= 1 ? 'Certain match' : 'Likely match'
}

/**
 * Whether keeping one side is guaranteed to lose nothing.
 *
 * Answered by the server (it compares the values in SQL); absent flags mean "unknown", and
 * unknown must read as unsafe — the quick button is the one that acts without showing what it
 * discards.
 */
export function canKeepA(dup: Pick<PotentialDuplicate, 'b_subset_of_a'>): boolean {
  return dup.b_subset_of_a === true
}

export function canKeepB(dup: Pick<PotentialDuplicate, 'a_subset_of_b'>): boolean {
  return dup.a_subset_of_b === true
}
