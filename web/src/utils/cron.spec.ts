import { describe, it, expect } from 'vitest'
import { humanizeCron, SYNC_PRESETS, BACKUP_PRESETS, DEDUP_PRESETS } from './cron'

describe('humanizeCron', () => {
  it.each([
    ['*/15 * * * *', 'Every 15 minutes'],
    ['*/1 * * * *', 'Every minute'],
    ['0 */6 * * *', 'Every 6 hours'],
    ['0 */1 * * *', 'Every hour'],
    ['0 * * * *', 'Every hour'],
    ['0 2 * * *', 'Daily at 2:00 AM'],
    ['0 0 * * *', 'Daily at midnight'],
    ['0 12 * * *', 'Daily at noon'],
    ['0 2 * * 0', 'Weekly on Sunday at 2:00 AM'],
    ['0 2 1 * *', 'Monthly on 1st at 2:00 AM'],
  ])('renders %s as %s', (expr, expected) => {
    expect(humanizeCron(expr)).toBe(expected)
  })

  it('falls back to the raw expression when it cannot describe it', () => {
    expect(humanizeCron('5 4 * * 1-5')).toBe('5 4 * * 1-5')
    expect(humanizeCron('not a cron')).toBe('not a cron')
    expect(humanizeCron('0 2 * *')).toBe('0 2 * *')
  })
})

describe('schedule presets', () => {
  it.each([
    ['sync', SYNC_PRESETS],
    ['backup', BACKUP_PRESETS],
    ['dedup', DEDUP_PRESETS],
  ])('%s presets end with a custom option', (_name, presets) => {
    expect(presets[presets.length - 1].value).toBe('custom')
  })

  it.each([
    ['sync', SYNC_PRESETS],
    ['backup', BACKUP_PRESETS],
    ['dedup', DEDUP_PRESETS],
  ])('%s presets carry cron expressions the humanizer understands', (_name, presets) => {
    for (const preset of presets.filter((p) => p.value !== 'custom')) {
      expect(humanizeCron(preset.value), `preset ${preset.label}`).not.toBe(preset.value)
    }
  })
})
