<template>
  <div>
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <div class="flex items-center gap-3">
        <h1 class="text-2xl font-bold text-gray-900">Contacts</h1>
        <span
          v-if="store.total > 0"
          class="inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-gray-100 text-gray-700"
        >
          {{ store.total }}
        </span>
      </div>
      <div class="flex items-center gap-3">
        <ViewToggle v-model="viewMode" />
        <AppButton variant="danger" @click="showDeleteAll = true">Delete All</AppButton>
        <RouterLink to="/contacts/new">
          <AppButton>Add Contact</AppButton>
        </RouterLink>
      </div>
    </div>

    <!-- Search + Filters -->
    <div class="space-y-3 mb-4">
      <SearchBar v-model="search" placeholder="Search contacts..." />
      <FilterBar
        v-if="store.facets"
        :categories="store.facets.categories"
        :orgs="store.facets.orgs"
        :selected-categories="store.filterCategory"
        :selected-org="store.filterOrg"
        :has-email="store.filterHasEmail"
        :has-phone="store.filterHasPhone"
        @update:categories="store.filterCategory = $event; resetPage()"
        @update:org="store.filterOrg = $event; resetPage()"
        @update:has-email="store.filterHasEmail = $event; resetPage()"
        @update:has-phone="store.filterHasPhone = $event; resetPage()"
        @clear="store.resetFilters(); resetPage()"
      />
    </div>

    <!-- Table view -->
    <AppCard v-if="viewMode === 'table'">
      <ContactTable
        :contacts="store.contacts"
        :loading="store.loading"
        :sort-by="store.sortBy"
        :sort-dir="store.sortDir"
        :selected-ids="selectedIds"
        :show-checkboxes="selectedIds.size > 0"
        @select="(c) => router.push({ name: 'contact-detail', params: { id: c.id } })"
        @delete="confirmDelete"
        @sort="handleSort"
        @toggle-select="toggleSelect"
        @select-all="toggleSelectAll"
      />
      <AppPagination
        :page="page"
        :per-page="store.perPage"
        :total="store.total"
        show-per-page
        @update:page="page = $event"
        @update:per-page="store.perPage = $event; resetPage()"
      />
    </AppCard>

    <!-- Card view -->
    <div v-else>
      <!-- Loading skeleton -->
      <div v-if="store.loading" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <div v-for="i in 6" :key="'skel-' + i" class="bg-white border border-gray-200 rounded-lg p-4 animate-pulse">
          <div class="flex items-start gap-3">
            <div class="h-12 w-12 bg-gray-200 rounded-full" />
            <div class="flex-1 space-y-2">
              <div class="h-4 w-32 bg-gray-200 rounded" />
              <div class="h-3 w-24 bg-gray-200 rounded" />
              <div class="h-3 w-40 bg-gray-200 rounded" />
            </div>
          </div>
        </div>
      </div>

      <!-- Empty state -->
      <div v-else-if="store.contacts.length === 0" class="text-center py-16">
        <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1">
          <path stroke-linecap="round" stroke-linejoin="round" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
        <h3 class="mt-2 text-sm font-medium text-gray-900">No contacts</h3>
        <p class="mt-1 text-sm text-gray-500">
          <template v-if="hasActiveFilters">
            No contacts match your filters.
            <button class="text-indigo-600 hover:text-indigo-800" @click="store.resetFilters(); resetPage()">Clear filters</button>
          </template>
          <template v-else>Get started by adding your first contact.</template>
        </p>
        <div v-if="!hasActiveFilters" class="mt-4">
          <RouterLink to="/contacts/new">
            <AppButton>Add Contact</AppButton>
          </RouterLink>
        </div>
      </div>

      <!-- Cards grid -->
      <div v-else class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
        <ContactCard
          v-for="contact in store.contacts"
          :key="contact.id"
          :contact="contact"
          :selected="selectedIds.has(contact.id)"
          :show-checkbox="selectedIds.size > 0"
          @select="(c) => router.push({ name: 'contact-detail', params: { id: c.id } })"
          @toggle-select="toggleSelect"
        />
      </div>

      <AppPagination
        class="mt-4"
        :page="page"
        :per-page="store.perPage"
        :total="store.total"
        show-per-page
        @update:page="page = $event"
        @update:per-page="store.perPage = $event; resetPage()"
      />
    </div>

    <!-- Bulk action bar -->
    <BulkActionBar
      :selected-count="selectedIds.size"
      :total-count="store.total"
      @select-all="selectAllVisible"
      @export="handleBulkExport"
      @delete="showBulkDelete = true"
      @close="selectedIds.clear()"
    />

    <!-- Delete single contact -->
    <ConfirmDialog
      :show="!!deleteTarget"
      title="Delete Contact"
      message="Are you sure you want to delete this contact? This action cannot be undone."
      confirm-text="Delete"
      :loading="deleting"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />

    <!-- Delete all contacts -->
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

    <!-- Bulk delete -->
    <ConfirmDialog
      :show="showBulkDelete"
      :title="`Delete ${selectedIds.size} contacts`"
      message="Are you sure you want to delete the selected contacts? This action cannot be undone."
      confirm-text="Delete"
      :loading="bulkDeleting"
      @confirm="handleBulkDelete"
      @cancel="showBulkDelete = false"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import { useContactsStore } from '@/stores/contacts'
import type { Contact } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import SearchBar from '@/components/ui/SearchBar.vue'
import ViewToggle from '@/components/ui/ViewToggle.vue'
import ContactTable from '@/components/contacts/ContactTable.vue'
import ContactCard from '@/components/contacts/ContactCard.vue'
import FilterBar from '@/components/contacts/FilterBar.vue'
import BulkActionBar from '@/components/contacts/BulkActionBar.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

const router = useRouter()
const store = useContactsStore()

const search = ref('')
const page = ref(1)
const deleteTarget = ref<Contact | null>(null)
const deleting = ref(false)
const showDeleteAll = ref(false)
const deletingAll = ref(false)
const showBulkDelete = ref(false)
const bulkDeleting = ref(false)
const selectedIds = reactive(new Set<string>())

// View mode: persisted in localStorage, auto-card on mobile
const viewMode = ref<'table' | 'card'>(
  (localStorage.getItem('contacts-view-mode') as 'table' | 'card') ??
  (window.innerWidth < 640 ? 'card' : 'table')
)
watch(viewMode, (v) => localStorage.setItem('contacts-view-mode', v))

const hasActiveFilters = computed(() =>
  store.filterCategory.length > 0 || !!store.filterOrg || store.filterHasEmail || store.filterHasPhone
)

function load() {
  store.fetchContacts({ page: page.value, search: search.value || undefined })
}

function resetPage() {
  page.value = 1
  load()
}

onMounted(() => {
  load()
  store.fetchFacets()
})

watch([page], load)
watch([search], () => {
  page.value = 1
  load()
})

// Sort handling
function handleSort(key: string) {
  if (store.sortBy === key) {
    store.sortDir = store.sortDir === 'asc' ? 'desc' : 'asc'
  } else {
    store.sortBy = key
    store.sortDir = 'asc'
  }
  resetPage()
}

// Selection
function toggleSelect(id: string) {
  if (selectedIds.has(id)) {
    selectedIds.delete(id)
  } else {
    selectedIds.add(id)
  }
}

function toggleSelectAll() {
  const allSelected = store.contacts.every(c => selectedIds.has(c.id))
  if (allSelected) {
    store.contacts.forEach(c => selectedIds.delete(c.id))
  } else {
    store.contacts.forEach(c => selectedIds.add(c.id))
  }
}

function selectAllVisible() {
  store.contacts.forEach(c => selectedIds.add(c.id))
}

// Delete handlers
function confirmDelete(contact: Contact) {
  deleteTarget.value = contact
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await store.deleteContact(deleteTarget.value.id)
    selectedIds.delete(deleteTarget.value.id)
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
    selectedIds.clear()
    showDeleteAll.value = false
    store.fetchFacets()
  } finally {
    deletingAll.value = false
  }
}

async function handleBulkDelete() {
  bulkDeleting.value = true
  try {
    const ids = [...selectedIds]
    for (const id of ids) {
      await store.deleteContact(id)
    }
    selectedIds.clear()
    showBulkDelete.value = false
    load()
    store.fetchFacets()
  } finally {
    bulkDeleting.value = false
  }
}

async function handleBulkExport() {
  // Export selected contacts as vCard
  const { getVCard } = await import('@/api/contacts')
  const vcards: string[] = []
  for (const id of selectedIds) {
    try {
      const { data } = await getVCard(id)
      vcards.push(data)
    } catch {
      // skip failed exports
    }
  }
  if (vcards.length === 0) return
  const blob = new Blob([vcards.join('\n')], { type: 'text/vcard' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `contacts-${selectedIds.size}.vcf`
  a.click()
  URL.revokeObjectURL(url)
}
</script>
