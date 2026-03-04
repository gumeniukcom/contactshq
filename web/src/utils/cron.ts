export interface SchedulePreset {
  label: string
  value: string
}

export const SYNC_PRESETS: SchedulePreset[] = [
  { label: 'Every 15 min',  value: '*/15 * * * *' },
  { label: 'Hourly',        value: '0 * * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Daily at 2 AM', value: '0 2 * * *' },
  { label: 'Custom',        value: 'custom' },
]

export const BACKUP_PRESETS: SchedulePreset[] = [
  { label: 'Daily at 2 AM',         value: '0 2 * * *' },
  { label: 'Daily at midnight',     value: '0 0 * * *' },
  { label: 'Weekly (Sun 2 AM)',     value: '0 2 * * 0' },
  { label: 'Monthly (1st at 2 AM)', value: '0 2 1 * *' },
  { label: 'Custom',                value: 'custom' },
]

export const DEDUP_PRESETS: SchedulePreset[] = [
  { label: 'Daily at 2 AM', value: '0 2 * * *' },
  { label: 'Weekly (Sun)',   value: '0 2 * * 0' },
  { label: 'Monthly (1st)',  value: '0 2 1 * *' },
  { label: 'Custom',         value: 'custom' },
]

/**
 * Convert a cron expression to a human-readable string.
 * Handles ~15 common patterns; falls back to the raw expression.
 */
export function humanizeCron(expr: string): string {
  const parts = expr.trim().split(/\s+/)
  if (parts.length !== 5) return expr

  const [min, hour, dom, mon, dow] = parts

  // Every N minutes
  if (min.startsWith('*/') && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    const n = parseInt(min.slice(2))
    return n === 1 ? 'Every minute' : `Every ${n} minutes`
  }

  // Every N hours
  if (min === '0' && hour.startsWith('*/') && dom === '*' && mon === '*' && dow === '*') {
    const n = parseInt(hour.slice(2))
    return n === 1 ? 'Every hour' : `Every ${n} hours`
  }

  // Every hour (on the hour)
  if (min === '0' && hour === '*' && dom === '*' && mon === '*' && dow === '*') {
    return 'Every hour'
  }

  // Daily at specific time
  if (/^\d+$/.test(min) && /^\d+$/.test(hour) && dom === '*' && mon === '*' && dow === '*') {
    return `Daily at ${fmtTime(hour, min)}`
  }

  // Weekly on specific day
  if (/^\d+$/.test(min) && /^\d+$/.test(hour) && dom === '*' && mon === '*' && /^\d+$/.test(dow)) {
    return `Weekly on ${dayName(dow)} at ${fmtTime(hour, min)}`
  }

  // Monthly on specific day
  if (/^\d+$/.test(min) && /^\d+$/.test(hour) && /^\d+$/.test(dom) && mon === '*' && dow === '*') {
    return `Monthly on ${ordinal(parseInt(dom))} at ${fmtTime(hour, min)}`
  }

  return expr
}

function fmtTime(h: string, m: string): string {
  const hh = parseInt(h)
  const mm = parseInt(m)
  if (hh === 0 && mm === 0) return 'midnight'
  if (hh === 12 && mm === 0) return 'noon'
  const suffix = hh >= 12 ? 'PM' : 'AM'
  const h12 = hh % 12 || 12
  return mm === 0 ? `${h12}:00 ${suffix}` : `${h12}:${String(mm).padStart(2, '0')} ${suffix}`
}

function dayName(d: string): string {
  return ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'][parseInt(d)] ?? d
}

function ordinal(n: number): string {
  const s = ['th', 'st', 'nd', 'rd']
  const v = n % 100
  return n + (s[(v - 20) % 10] || s[v] || s[0])
}
