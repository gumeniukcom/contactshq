import client from './client'
import type { TokenPair, LoginInput } from '@/types'

export function login(data: LoginInput) {
  return client.post<TokenPair>('/auth/login', data)
}

export function refresh(refreshToken: string) {
  return client.post<TokenPair>('/auth/refresh', { refresh_token: refreshToken })
}
