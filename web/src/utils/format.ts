/**
 * Small pure formatters shared across screens.
 *
 * They lived inside `<script setup>` blocks — formatSize in BackupView, formatDuration in
 * PipelineViewView — where nothing could test them and each screen was free to drift into its
 * own idea of what a megabyte looks like.
 */

/** Renders a byte count for a human. */
export function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '—'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`
  return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB`
}

/**
 * Renders how long something took, given its start and end.
 *
 * A missing end means it is still going — which is a real state for a run whose process died,
 * so it must render as something rather than as a nonsense duration.
 */
export function formatDuration(startedAt: string, finishedAt?: string | null): string {
  if (!finishedAt) return 'running…'

  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime()
  if (!Number.isFinite(ms) || ms < 0) return '—'
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`

  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return `${minutes}m ${seconds}s`
}

/** Renders an age as a rough, readable interval. */
export function formatAgo(iso: string, now: Date = new Date()): string {
  const ms = now.getTime() - new Date(iso).getTime()
  if (!Number.isFinite(ms)) return '—'
  if (ms < 0) return 'just now'

  const minutes = Math.floor(ms / 60_000)
  if (minutes < 1) return 'just now'
  if (minutes < 60) return `${minutes}m ago`

  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`

  const days = Math.floor(hours / 24)
  return days === 1 ? 'yesterday' : `${days} days ago`
}
