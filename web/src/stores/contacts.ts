import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '@/api/contacts'
import type { ContactFacets } from '@/api/contacts'
import type { Contact, CreateContactInput } from '@/types'

export const useContactsStore = defineStore('contacts', () => {
  const contacts = ref<Contact[]>([])
  const total = ref(0)
  const loading = ref(false)

  // Sort & filter state
  const sortBy = ref('name')
  const sortDir = ref<'asc' | 'desc'>('asc')
  const filterCategory = ref<string[]>([])
  const filterOrg = ref('')
  const filterHasEmail = ref(false)
  const filterHasPhone = ref(false)
  const perPage = ref(20)
  const facets = ref<ContactFacets | null>(null)

  async function fetchContacts(params?: { page?: number; search?: string }) {
    loading.value = true
    try {
      const pp = perPage.value
      const page = params?.page ?? 1
      const apiParams: api.ListContactsParams = {
        limit: pp,
        offset: (page - 1) * pp,
        q: params?.search || undefined,
        sort_by: sortBy.value,
        sort_dir: sortDir.value,
      }
      if (filterCategory.value.length > 0) {
        apiParams.category = filterCategory.value.join(',')
      }
      if (filterOrg.value) {
        apiParams.org = filterOrg.value
      }
      if (filterHasEmail.value) {
        apiParams.has_email = 'true'
      }
      if (filterHasPhone.value) {
        apiParams.has_phone = 'true'
      }
      const { data } = await api.listContacts(apiParams)
      contacts.value = data.contacts
      total.value = data.total
    } finally {
      loading.value = false
    }
  }

  async function fetchFacets() {
    const { data } = await api.getContactFacets()
    facets.value = data
  }

  function resetFilters() {
    filterCategory.value = []
    filterOrg.value = ''
    filterHasEmail.value = false
    filterHasPhone.value = false
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

  return {
    contacts, total, loading,
    sortBy, sortDir, filterCategory, filterOrg, filterHasEmail, filterHasPhone, perPage, facets,
    fetchContacts, fetchFacets, resetFilters,
    createContact, updateContact, deleteContact, deleteAllContacts,
  }
})
