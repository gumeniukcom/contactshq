import { describe, it, expect } from 'vitest'
import { backupHealth, schedulePeriodMs } from './backup-health'
import type { BackupRun } from '@/types'

const NOW = new Date('2026-08-06T12:00:00Z')

function run(overrides: Partial<BackupRun> = {}): BackupRun {
  return {
    id: 'r1',
    user_id: 'u1',
    trigger: 'scheduled',
    status: 'completed',
    size_bytes: 1024,
    contact_count: 10,
    compressed: false,
    started_at: '2026-08-06T02:00:00Z',
    finished_at: '2026-08-06T02:00:05Z',
    ...overrides,
  }
}

const daily = { enabled: true, schedule: '0 2 * * *' }

describe('backupHealth', () => {
  it('is healthy when the last run succeeded recently', () => {
    const health = backupHealth(daily, run(), run(), NOW)

    expect(health.status).toBe('healthy')
    expect(health.alarming).toBe(false)
  })

  // The interesting fact is that the most recent attempt failed, even if an older one worked.
  it('is failing when the latest attempt failed, despite an earlier success', () => {
    const failed = run({ status: 'failed', error_message: 'disk full' })
    const health = backupHealth(daily, run(), failed, NOW)

    expect(health.status).toBe('failing')
    expect(health.alarming).toBe(true)
    expect(health.error).toBe('disk full')
  })

  // A killed container leaves a run "interrupted"; that is a failure the user should see.
  it('treats an interrupted run as failing and says what happened', () => {
    const interrupted = run({ status: 'interrupted', error_message: 'server restarted' })
    const health = backupHealth(daily, null, interrupted, NOW)

    expect(health.status).toBe('failing')
    expect(health.summary).toContain('interrupted')
  })

  it('is overdue when the last success is older than two schedule periods', () => {
    const old = run({ started_at: '2026-08-03T02:00:00Z' }) // three days back
    const health = backupHealth(daily, old, old, NOW)

    expect(health.status).toBe('overdue')
    expect(health.alarming).toBe(true)
  })

  // One period would fire on ordinary jitter, and a status that cries wolf gets ignored.
  it('tolerates a single missed period', () => {
    const yesterday = run({ started_at: '2026-08-05T02:00:00Z' })
    const health = backupHealth(daily, yesterday, yesterday, NOW)

    expect(health.status).toBe('healthy')
  })

  it('reports never when the schedule is on but nothing has succeeded', () => {
    const health = backupHealth(daily, null, null, NOW)

    expect(health.status).toBe('never')
    expect(health.alarming).toBe(true)
  })

  // Backups the user switched off are not broken.
  it('is disabled, and not alarming, when scheduling is off', () => {
    const health = backupHealth({ enabled: false, schedule: '0 2 * * *' }, null, null, NOW)

    expect(health.status).toBe('disabled')
    expect(health.alarming).toBe(false)
  })

  it('treats missing settings as disabled rather than broken', () => {
    expect(backupHealth(null, null, null, NOW).status).toBe('disabled')
    expect(backupHealth(undefined, null, null, NOW).status).toBe('disabled')
  })

  it('describes the cadence in the summary', () => {
    expect(backupHealth(daily, run(), run(), NOW).summary).toMatch(/daily|02:00|day/i)
  })
})

describe('schedulePeriodMs', () => {
  it.each([
    ['every 15 minutes', '*/15 * * * *', 15 * 60_000],
    ['hourly', '0 * * * *', 3600_000],
    ['daily', '0 2 * * *', 24 * 3600_000],
    ['weekly', '0 2 * * 1', 7 * 24 * 3600_000],
    ['monthly', '0 2 1 * *', 30 * 24 * 3600_000],
  ])('derives the period of a %s schedule', (_name, expr, expected) => {
    expect(schedulePeriodMs(expr)).toBe(expected)
  })

  it('falls back to a day for an expression it cannot read', () => {
    expect(schedulePeriodMs('nonsense')).toBe(24 * 3600_000)
  })
})
