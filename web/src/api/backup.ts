import client from './client'
import type { BackupInfo, BackupRun, BackupSettings, BackupStatus, RestoreResult } from '@/types'

export function createBackup() {
  return client.post<BackupInfo>('/backup/create')
}

export function listBackups() {
  return client.get<{ backups: BackupInfo[] }>('/backup/list')
}

export function downloadBackup(id: string) {
  return client.get(`/backup/download/${encodeURIComponent(id)}`, { responseType: 'blob' })
}

export function deleteBackup(id: string) {
  return client.delete(`/backup/${encodeURIComponent(id)}`)
}

export function restoreBackup(id: string, mode: 'merge' | 'replace') {
  return client.post<RestoreResult>(`/backup/restore/${encodeURIComponent(id)}`, null, { params: { mode } })
}

export function getBackupSettings() {
  return client.get<BackupSettings>('/backup/settings')
}

export function saveBackupSettings(data: BackupSettings) {
  return client.put<BackupSettings>('/backup/settings', data)
}

/** The user's backup history, newest first. */
export function listBackupRuns(limit = 20) {
  return client.get<{ runs: BackupRun[] }>('/backup/runs', { params: { limit } })
}

/** Last success, last attempt and the next scheduled run in one call. */
export function getBackupStatus() {
  return client.get<BackupStatus>('/backup/status')
}
