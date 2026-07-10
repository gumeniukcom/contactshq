import type { PipelineStep, SyncDirection } from '@/types'

/** Name of the internal address book as users see it. */
export const LOCAL_LABEL = 'ContactsHQ'

export function providerLabel(type: string): string {
  switch (type) {
    case 'google':
      return 'Google'
    case 'carddav':
      return 'CardDAV'
    case 'internal':
      return LOCAL_LABEL
    default:
      return type
  }
}

/**
 * A step always pairs an external provider with the internal address book, so the flow
 * is described by naming the two sides and the arrow between them. The provider is on
 * the left for an import and on the right for an export, which is what the user reads
 * as "where the contacts come from".
 */
export function flowLeft(step: Pick<PipelineStep, 'source_type' | 'direction'>): string {
  return step.direction === 'export' ? LOCAL_LABEL : providerLabel(step.source_type)
}

export function flowRight(step: Pick<PipelineStep, 'source_type' | 'direction'>): string {
  return step.direction === 'export' ? providerLabel(step.source_type) : LOCAL_LABEL
}

export function flowArrow(step: Pick<PipelineStep, 'direction'>): string {
  return step.direction === 'two_way' ? '⇄' : '→'
}

export function directionLabel(direction: SyncDirection): string {
  switch (direction) {
    case 'export':
      return 'Export'
    case 'two_way':
      return 'Two-way'
    default:
      return 'Import'
  }
}
