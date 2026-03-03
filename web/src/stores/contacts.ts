import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '@/api/contacts'
import type { Contact, CreateContactInput } from '@/types'

export const useContactsStore = defineStore('contacts', () => {
  const contacts = ref<Contact[]>([])
  const total = ref(0)
  const loading = ref(false)

  async function fetchContacts(params?: { page?: number; per_page?: number; search?: string }) {
    loading.value = true
    try {
      const perPage = params?.per_page ?? 50
      const page = params?.page ?? 1
      const { data } = await api.listContacts({
        limit: perPage,
        offset: (page - 1) * perPage,
        q: params?.search,
      })
      contacts.value = data.contacts
      total.value = data.total
    } finally {
      loading.value = false
    }
  }

  async function createContact(input: Partial<CreateContactInput>) {
    const { data } = await api.createContact(input as CreateContactInput)
    return data
  }

  async function updateContact(id: string, input: Partial<CreateContactInput>) {
    const { data } = await api.updateContact(id, input)
    return data
  }

  async function deleteContact(id: string) {
    await api.deleteContact(id)
  }

  async function deleteAllContacts() {
    await api.deleteAllContacts()
    contacts.value = []
    total.value = 0
  }

  return { contacts, total, loading, fetchContacts, createContact, updateContact, deleteContact, deleteAllContacts }
})
