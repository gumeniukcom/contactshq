import client from './client'
import type { User, RegisterInput, UpdateRoleInput } from '@/types'

export function listUsers() {
  return client.get<{ users: User[]; total: number; limit: number; offset: number }>('/admin/users')
}

export function createUser(data: RegisterInput) {
  return client.post<User>('/admin/users', data)
}

export function updateUserRole(id: string, data: UpdateRoleInput) {
  return client.put<User>(`/admin/users/${id}/role`, data)
}

export function deleteUser(id: string) {
  return client.delete(`/admin/users/${id}`)
}
