import type { FieldDiff } from '@/types'

/**
 * Parses the `field_diffs` column of a sync conflict.
 *
 * The guard is not defensive programming for its own sake. Rows written before the server
 * stopped marshalling a nil slice hold the literal four bytes `null`, and `JSON.parse('null')`
 * returns `null` **without throwing** — so a bare try/catch does not catch it, and the caller
 * ends up invoking `.forEach` on null. That blanked the conflict detail page, and one such row
 * blanked the whole conflicts list.
 *
 * Anything that is not an array becomes an empty one: a conflict with no attributable
 * field-level diffs is a real, ordinary state (two sides edited different properties), and the
 * screens already render whole-card actions for it.
 */
export function parseFieldDiffs(raw: string | null | undefined): FieldDiff[] {
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? (parsed as FieldDiff[]) : []
  } catch {
    return []
  }
}
