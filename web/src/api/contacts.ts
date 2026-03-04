import client from './client'
import type { Contact, CreateContactInput, PotentialDuplicate, MergeInput } from '@/types'

export interface ListContactsParams {
  limit?: number
  offset?: number
  q?: string
  sort_by?: string
  sort_dir?: string
  category?: string
  org?: string
  has_email?: string
  has_phone?: string
}

export function listContacts(params?: ListContactsParams) {
  return client.get<{ contacts: Contact[]; total: number }>('/contacts', { params })
}

export interface ContactFacets {
  categories: string[]
  orgs: string[]
  total: number
}

export function getContactFacets() {
  return client.get<ContactFacets>('/contacts/facets')
}

export function getContact(id: string) {
  return client.get<Contact>(`/contacts/${id}`)
}

export function createContact(data: CreateContactInput) {
  return client.post<Contact>('/contacts', data)
}

export function updateContact(id: string, data: Partial<CreateContactInput>) {
  return client.put<Contact>(`/contacts/${id}`, data)
}

export function deleteContact(id: string) {
  return client.delete(`/contacts/${id}`)
}

export function deleteAllContacts() {
  return client.delete('/contacts/')
}

export function getVCard(id: string) {
  return client.get<string>(`/contacts/${id}/vcard`, { responseType: 'text' as never })
}

export function getQRCode(id: string) {
  return client.get(`/contacts/${id}/qrcode`, { responseType: 'arraybuffer' })
}

export function listDuplicates(params?: { status?: string; limit?: number; offset?: number }) {
  return client.get<{ duplicates: PotentialDuplicate[]; total: number }>('/contacts/duplicates', { params })
}

export function countDuplicates() {
  return client.get<{ pending: number }>('/contacts/duplicates/count')
}

export function detectDuplicates() {
  return client.post<{ found: number; checked: number }>('/contacts/duplicates/detect')
}

export function dismissDuplicate(id: string) {
  return client.post<{ message: string }>(`/contacts/duplicates/${id}/dismiss`)
}

export function mergeContacts(data: MergeInput) {
  return client.post<Contact>('/contacts/merge', data)
}

