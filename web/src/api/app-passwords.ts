import client from './client'
import type { AppPassword } from '@/types'

export function listAppPasswords() {
  return client.get<{ app_passwords: AppPassword[] }>('/app-passwords')
}

export function createAppPassword(label: string) {
  return client.post<{ id: string; label: string; token: string; created_at: string }>('/app-passwords', { label })
}

export function deleteAppPassword(id: string) {
  return client.delete(`/app-passwords/${id}`)
}
