import client from './client'
import type { ImportResult } from '@/types'

export function importVCard(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return client.post<ImportResult>('/import/vcard', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function importCSV(file: File) {
  const formData = new FormData()
  formData.append('file', file)
  return client.post<ImportResult>('/import/csv', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

export function exportVCard() {
  return client.get('/export/vcard', { responseType: 'blob' })
}

export function exportCSV() {
  return client.get('/export/csv', { responseType: 'blob' })
}

export function exportJSON() {
  return client.get('/export/json', { responseType: 'blob' })
}
