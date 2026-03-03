<template>
  <div class="max-w-3xl space-y-6">
    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold text-gray-900">Credentials</h1>
      <AppButton @click="openAdd">+ Add Credential</AppButton>
    </div>

    <!-- List -->
    <AppCard>
      <div v-if="loading" class="text-sm text-gray-400 py-4 text-center">Loading…</div>

      <div v-else-if="credentials.length === 0" class="text-sm text-gray-500 py-6 text-center">
        No credentials yet. Click <strong>+ Add Credential</strong> to add your first connection.
      </div>

      <table v-else class="w-full text-sm">
        <thead>
          <tr class="border-b border-gray-100 text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">
            <th class="pb-2 pr-4">Name</th>
            <th class="pb-2 pr-4">Type</th>
            <th class="pb-2 pr-4">Server URL</th>
            <th class="pb-2 text-right">Actions</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-50">
          <tr v-for="cred in credentials" :key="cred.id" class="hover:bg-gray-50">
            <td class="py-3 pr-4 font-medium text-gray-900">{{ cred.name }}</td>
            <td class="py-3 pr-4">
              <AppBadge color="blue">{{ cred.provider_type }}</AppBadge>
            </td>
            <td class="py-3 pr-4 text-gray-500 truncate max-w-xs">{{ cred.endpoint }}</td>
            <td class="py-3 text-right space-x-2 whitespace-nowrap">
              <AppButton size="sm" variant="secondary" @click="openEdit(cred)">Edit</AppButton>
              <AppButton size="sm" variant="danger" :loading="deletingId === cred.id" @click="handleDelete(cred)">Delete</AppButton>
            </td>
          </tr>
        </tbody>
      </table>
    </AppCard>

    <!-- Add / Edit modal -->
    <div v-if="modal.open" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
      <div class="bg-white rounded-xl shadow-xl w-full max-w-md">
        <div class="flex items-center justify-between px-6 pt-5 pb-4 border-b border-gray-100">
          <h2 class="text-base font-semibold text-gray-900">{{ modal.id ? 'Edit Credential' : 'Add Credential' }}</h2>
          <button class="text-gray-400 hover:text-gray-600" @click="closeModal">
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>

        <form class="px-6 py-4 space-y-4" @submit.prevent="handleSave">
          <AppInput v-model="modal.form.name" label="Name" placeholder="Fastmail CardDAV" id="cred-name" />

          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Type</label>
            <select
              v-model="modal.form.provider_type"
              class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            >
              <option value="carddav">CardDAV</option>
              <option value="google" disabled>Google (coming soon)</option>
            </select>
          </div>

          <AppInput
            v-model="modal.form.endpoint"
            label="Server URL"
            placeholder="https://dav.example.com/addressbooks/user/"
            id="cred-url"
          />

          <AppInput v-model="modal.form.username" label="Username" id="cred-user" />

          <AppInput
            v-model="modal.form.password"
            label="Password"
            type="password"
            :placeholder="modal.id ? 'Leave blank to keep current' : ''"
            id="cred-pass"
          />

          <div class="flex items-center gap-2">
            <input
              id="cred-tls"
              v-model="modal.form.skip_tls_verify"
              type="checkbox"
              class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
            />
            <label for="cred-tls" class="text-sm text-gray-700">
              Skip TLS/SSL certificate verification
              <span class="text-xs text-gray-400">(use for self-signed certs)</span>
            </label>
          </div>

          <p v-if="modal.error" class="text-sm text-red-600">{{ modal.error }}</p>

          <div class="flex justify-end gap-3 pt-2">
            <AppButton variant="secondary" type="button" @click="closeModal">Cancel</AppButton>
            <AppButton type="submit" :loading="modal.saving">
              {{ modal.id ? 'Update' : 'Create' }}
            </AppButton>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, reactive } from 'vue'
import type { Credential } from '@/types'
import { listCredentials, createCredential, updateCredential, deleteCredential } from '@/api/credentials'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppInput from '@/components/ui/AppInput.vue'

// ── List ───────────────────────────────────────────────────────────────────
const credentials = ref<Credential[]>([])
const loading = ref(true)

async function loadCredentials() {
  loading.value = true
  try {
    const { data } = await listCredentials()
    credentials.value = data.credentials ?? []
  } finally {
    loading.value = false
  }
}

onMounted(loadCredentials)

// ── Delete ─────────────────────────────────────────────────────────────────
const deletingId = ref<string | null>(null)

async function handleDelete(cred: Credential) {
  if (!confirm(`Delete credential "${cred.name}"?`)) return
  deletingId.value = cred.id
  try {
    await deleteCredential(cred.id)
    await loadCredentials()
  } finally {
    deletingId.value = null
  }
}

// ── Modal ──────────────────────────────────────────────────────────────────
interface ModalForm {
  name: string
  provider_type: string
  endpoint: string
  username: string
  password: string
  skip_tls_verify: boolean
}

function emptyForm(): ModalForm {
  return { name: '', provider_type: 'carddav', endpoint: '', username: '', password: '', skip_tls_verify: false }
}

const modal = reactive({
  open: false,
  id: null as string | null,
  form: emptyForm(),
  saving: false,
  error: '',
})

function openAdd() {
  modal.open = true
  modal.id = null
  Object.assign(modal.form, emptyForm())
  modal.error = ''
}

function openEdit(cred: Credential) {
  modal.open = true
  modal.id = cred.id
  Object.assign(modal.form, {
    name: cred.name,
    provider_type: cred.provider_type,
    endpoint: cred.endpoint,
    username: cred.username,
    password: '',
    skip_tls_verify: cred.skip_tls_verify,
  })
  modal.error = ''
}

function closeModal() {
  modal.open = false
}

async function handleSave() {
  modal.saving = true
  modal.error = ''
  try {
    if (modal.id) {
      await updateCredential(modal.id, modal.form)
    } else {
      await createCredential(modal.form)
    }
    closeModal()
    await loadCredentials()
  } catch (e: unknown) {
    modal.error = (e as Error)?.message ?? 'Failed to save credential'
  } finally {
    modal.saving = false
  }
}
</script>
