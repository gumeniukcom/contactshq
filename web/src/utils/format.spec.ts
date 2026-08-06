import { describe, it, expect } from 'vitest'
import { formatAgo, formatDuration, formatSize } from './format'

describe('formatSize', () => {
  it.each([
    [0, '0 B'],
    [512, '512 B'],
    [1024, '1.0 KB'],
    [1536, '1.5 KB'],
    [1024 * 1024, '1.0 MB'],
    [1024 * 1024 * 1024, '1.0 GB'],
  ])('renders %i bytes as %s', (bytes, expected) => {
    expect(formatSize(bytes)).toBe(expected)
  })

  it('refuses to invent a number for nonsense input', () => {
    expect(formatSize(-1)).toBe('—')
    expect(formatSize(NaN)).toBe('—')
  })
})

describe('formatDuration', () => {
  it('renders sub-second work in milliseconds', () => {
    expect(formatDuration('2026-08-06T02:00:00Z', '2026-08-06T02:00:00.400Z')).toBe('400ms')
  })

  it('renders seconds', () => {
    expect(formatDuration('2026-08-06T02:00:00Z', '2026-08-06T02:00:05Z')).toBe('5.0s')
  })

  it('renders minutes and seconds', () => {
    expect(formatDuration('2026-08-06T02:00:00Z', '2026-08-06T02:03:20Z')).toBe('3m 20s')
  })

  // A run whose process died has no end. That is a real state, not a bug.
  it('says a run without an end is still going', () => {
    expect(formatDuration('2026-08-06T02:00:00Z')).toBe('running…')
    expect(formatDuration('2026-08-06T02:00:00Z', null)).toBe('running…')
  })

  it('does not render a negative duration', () => {
    expect(formatDuration('2026-08-06T02:00:05Z', '2026-08-06T02:00:00Z')).toBe('—')
  })
})

describe('formatAgo', () => {
  const now = new Date('2026-08-06T12:00:00Z')

  it.each([
    ['2026-08-06T11:59:30Z', 'just now'],
    ['2026-08-06T11:30:00Z', '30m ago'],
    ['2026-08-06T09:00:00Z', '3h ago'],
    ['2026-08-05T12:00:00Z', 'yesterday'],
    ['2026-08-01T12:00:00Z', '5 days ago'],
  ])('renders %s as %s', (iso, expected) => {
    expect(formatAgo(iso, now)).toBe(expected)
  })

  // Clock skew between the server and the browser must not produce "in -3 minutes".
  it('handles a timestamp in the future', () => {
    expect(formatAgo('2026-08-06T12:05:00Z', now)).toBe('just now')
  })
})
