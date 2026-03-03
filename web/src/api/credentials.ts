import client from './client'
import type { Credential } from '@/types'

export interface CredentialInput {
  name: string
  provider_type: string
  endpoint: string
  username: string
  password: string
  skip_tls_verify: boolean
}

export function listCredentials(params?: { type?: string }) {
  return client.get<{ credentials: Credential[] }>('/credentials', { params })
}

export function createCredential(data: CredentialInput) {
  return client.post<Credential>('/credentials', data)
}

export function getCredential(id: string) {
  return client.get<Credential>(`/credentials/${id}`)
}

export function updateCredential(id: string, data: Partial<CredentialInput>) {
  return client.put<Credential>(`/credentials/${id}`, data)
}

export function deleteCredential(id: string) {
  return client.delete(`/credentials/${id}`)
}
