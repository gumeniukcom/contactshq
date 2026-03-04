import client from './client'

export interface GoogleInitRequest {
  client_id: string
  client_secret: string
  redirect_url?: string
}

export interface GoogleInitResponse {
  auth_url: string
}

export interface GoogleStatus {
  connected: boolean
  name?: string
  created_at?: string
  last_sync_at?: string
  last_error?: string
}

export function initGoogleAuth(data: GoogleInitRequest) {
  return client.post<GoogleInitResponse>('/auth/google/init', data)
}

export function getGoogleStatus() {
  return client.get<GoogleStatus>('/auth/google/status')
}

export function disconnectGoogle() {
  return client.delete('/auth/google/disconnect')
}
