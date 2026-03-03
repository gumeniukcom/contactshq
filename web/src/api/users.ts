import client from './client'
import type { User, UpdateProfileInput, ChangePasswordInput } from '@/types'

export function getMe() {
  return client.get<User>('/users/me')
}

export function updateProfile(data: UpdateProfileInput) {
  return client.put<User>('/users/me', data)
}

export function changePassword(data: ChangePasswordInput) {
  return client.put('/users/me/password', data)
}
