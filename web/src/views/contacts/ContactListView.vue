<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-gray-900">Contacts</h1>
      <div class="flex items-center gap-3">
        <AppButton variant="danger" @click="showDeleteAll = true">Delete All</AppButton>
        <RouterLink to="/contacts/new">
          <AppButton>Add Contact</AppButton>
        </RouterLink>
      </div>
    </div>

    <div class="mb-4">
      <SearchBar v-model="search" placeholder="Search contacts..." />
    </div>

    <AppCard>
      <ContactTable
        :contacts="store.contacts"
        :loading="store.loading"
        @select="(c) => router.push({ name: 'contact-detail', params: { id: c.id } })"
        @delete="confirmDelete"
      />
      <AppPagination
        :page="page"
        :per-page="perPage"
        :total="store.total"
        @update:page="page = $event"
      />
    </AppCard>

    <ConfirmDialog
      :show="!!deleteTarget"
      title="Delete Contact"
      message="Are you sure you want to delete this contact? This action cannot be undone."
      confirm-text="Delete"
      :loading="deleting"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />

    <ConfirmDialog
      :show="showDeleteAll"
      title="Delete All Contacts"
      message="This will permanently delete every contact in your address book. This action cannot be undone."
      confirm-text="Delete All"
      require-text="delete"
      :loading="deletingAll"
      @confirm="handleDeleteAll"
      @cancel="showDeleteAll = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useContactsStore } from '@/stores/contacts'
import type { Contact } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import SearchBar from '@/components/ui/SearchBar.vue'
import ContactTable from '@/components/contacts/ContactTable.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

const router = useRouter()
const store = useContactsStore()

const search = ref('')
const page = ref(1)
const perPage = 20
const deleteTarget = ref<Contact | null>(null)
const deleting = ref(false)
const showDeleteAll = ref(false)
const deletingAll = ref(false)

function load() {
  store.fetchContacts({ page: page.value, per_page: perPage, search: search.value || undefined })
}

onMounted(load)
watch([page, search], () => {
  if (search.value !== '') page.value = 1
  load()
})

function confirmDelete(contact: Contact) {
  deleteTarget.value = contact
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await store.deleteContact(deleteTarget.value.id)
    deleteTarget.value = null
    load()
  } finally {
    deleting.value = false
  }
}

async function handleDeleteAll() {
  deletingAll.value = true
  try {
    await store.deleteAllContacts()
    showDeleteAll.value = false
  } finally {
    deletingAll.value = false
  }
}
</script>
