import client from './client'
import type { SyncProvider, SyncConflict } from '@/types'

export interface ConflictsResponse {
  conflicts: SyncConflict[]
  total: number
}

export function listProviders() {
  return client.get<{ providers: SyncProvider[] }>('/sync/providers')
}

export function connectCardDAV(data: { url: string; username: string; password: string; skip_tls_verify?: boolean }) {
  return client.post<{ message: string; id: string }>('/sync/carddav/connect', data)
}

export function triggerCardDAVSync() {
  return client.post<{ message: string }>('/sync/carddav/trigger')
}

export function disconnectProvider(id: string) {
  return client.delete(`/sync/providers/${id}`)
}

export function listConflicts(params?: { status?: string; limit?: number; offset?: number }) {
  return client.get<ConflictsResponse>('/sync/conflicts', { params })
}

export function countConflicts() {
  return client.get<{ pending: number }>('/sync/conflicts/count')
}

export function getConflict(id: string) {
  return client.get<SyncConflict>(`/sync/conflicts/${id}`)
}

export function resolveConflict(id: string, resolution: Record<string, string>) {
  return client.post<{ message: string; resolved_vcard: string }>(`/sync/conflicts/${id}/resolve`, { resolution })
}

export function dismissConflict(id: string) {
  return client.post<{ message: string }>(`/sync/conflicts/${id}/dismiss`)
}
