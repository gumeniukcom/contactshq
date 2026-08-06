import type { BackupRun, BackupSettings } from '@/types'
import { humanizeCron } from './cron'

/**
 * "Is my backup working?" answered in one word.
 *
 * The failure this exists for is silent: a scheduled backup starts failing, nothing surfaces
 * it, and the problem is discovered on the day the backup is needed. Counting files on disk
 * cannot answer it either — retention deletes them, and at retention=1 the only file present
 * is always the newest one.
 */
export type BackupHealthStatus = 'healthy' | 'failing' | 'overdue' | 'never' | 'disabled'

export interface BackupHealth {
  status: BackupHealthStatus
  /** One line for a person, already phrased. */
  summary: string
  /** The error from the most recent failed attempt, when there is one. */
  error?: string
  /** True for the states that deserve a red banner rather than a grey one. */
  alarming: boolean
}

/**
 * Two schedule periods before a missed backup counts as overdue.
 *
 * One period would fire on every ordinary jitter — a nightly job that runs at 02:05 instead
 * of 02:00 is not a problem, and a status that cries wolf gets ignored.
 */
const OVERDUE_PERIODS = 2

/** Rough length of one schedule period, derived from the cron expression's shape. */
export function schedulePeriodMs(schedule: string): number {
  const parts = schedule.trim().split(/\s+/)
  if (parts.length !== 5) return 24 * 3600_000

  const [min, hour, dom, mon, dow] = parts

  if (min.startsWith('*/')) return Math.max(1, Number(min.slice(2)) || 1) * 60_000
  if (hour === '*') return 3600_000
  if (dom === '*' && mon === '*' && dow === '*') return 24 * 3600_000
  if (dow !== '*') return 7 * 24 * 3600_000
  if (dom !== '*') return 30 * 24 * 3600_000
  return 24 * 3600_000
}

export function backupHealth(
  settings: Pick<BackupSettings, 'enabled' | 'schedule'> | null | undefined,
  lastSuccess: BackupRun | null | undefined,
  lastRun: BackupRun | null | undefined,
  now: Date = new Date(),
): BackupHealth {
  // Backups the user switched off are not broken.
  if (!settings?.enabled) {
    return {
      status: 'disabled',
      summary: 'Scheduled backups are off.',
      alarming: false,
    }
  }

  const cadence = humanizeCron(settings.schedule)

  // A failing latest attempt outranks an older success: the interesting fact is that the last
  // thing that happened went wrong.
  if (lastRun && (lastRun.status === 'failed' || lastRun.status === 'interrupted')) {
    return {
      status: 'failing',
      summary:
        lastRun.status === 'interrupted'
          ? 'The last backup was interrupted — the server stopped while it was running.'
          : 'The last backup failed.',
      error: lastRun.error_message || undefined,
      alarming: true,
    }
  }

  if (!lastSuccess) {
    return {
      status: 'never',
      summary: `No backup has succeeded yet. Scheduled ${cadence.toLowerCase()}.`,
      alarming: true,
    }
  }

  const age = now.getTime() - new Date(lastSuccess.started_at).getTime()
  if (age > schedulePeriodMs(settings.schedule) * OVERDUE_PERIODS) {
    return {
      status: 'overdue',
      summary: `The last successful backup is older than the schedule allows (${cadence.toLowerCase()}).`,
      alarming: true,
    }
  }

  return {
    status: 'healthy',
    summary: `Backups are running ${cadence.toLowerCase()}.`,
    alarming: false,
  }
}
