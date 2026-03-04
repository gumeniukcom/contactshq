<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-foreground mb-2">App Passwords</h1>
    <p class="text-muted-foreground mb-6">
      Create app-specific passwords for CardDAV clients (iPhone, macOS Contacts, Thunderbird).
      These are separate from your main password and can be revoked individually.
    </p>

    <!-- Create -->
    <AppCard class="mb-6">
      <h2 class="text-lg font-semibold text-foreground mb-4">Create App Password</h2>
      <form class="flex gap-3 items-end" @submit.prevent="handleCreate">
        <div class="flex-1">
          <AppInput
            v-model="newLabel"
            label="Label"
            placeholder="e.g. iPhone, Thunderbird"
            id="app-pw-label"
          />
        </div>
        <AppButton type="submit" :loading="creating" :disabled="!newLabel.trim()">
          Create
        </AppButton>
      </form>
    </AppCard>

    <!-- Token display (shown once after creation) -->
    <div v-if="generatedToken" class="mb-6 p-4 bg-green-50 dark:bg-green-500/20 border border-green-200 dark:border-green-500/30 rounded-lg">
      <p class="font-medium text-green-800 dark:text-green-300 mb-2">App password created!</p>
      <p class="text-sm text-green-700 dark:text-green-400 mb-3">
        Copy this password now — it won't be shown again.
      </p>
      <div class="flex items-center gap-2">
        <code class="flex-1 px-3 py-2 bg-white dark:bg-black/30 border border-green-300 dark:border-green-500/40 rounded font-mono text-sm text-foreground select-all break-all">{{ generatedToken }}</code>
        <AppButton variant="secondary" size="sm" @click="copyToken">
          {{ copied ? 'Copied!' : 'Copy' }}
        </AppButton>
      </div>
    </div>

    <!-- List -->
    <div v-if="loading" class="text-muted-foreground">Loading...</div>

    <div v-else-if="passwords.length === 0" class="text-muted-foreground text-sm">
      No app passwords yet. Create one to use with CardDAV clients.
    </div>

    <div v-else class="space-y-3">
      <AppCard v-for="pw in passwords" :key="pw.id" class="flex items-center justify-between">
        <div>
          <p class="font-medium text-foreground">{{ pw.label }}</p>
          <p class="text-sm text-muted-foreground">
            Created {{ formatDateTime(pw.created_at) }}
            <span v-if="pw.last_used_at"> · Last used {{ formatDateTime(pw.last_used_at) }}</span>
            <span v-else> · Never used</span>
          </p>
        </div>
        <AppButton variant="danger" size="sm" @click="handleDelete(pw.id, pw.label)">
          Delete
        </AppButton>
      </AppCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listAppPasswords, createAppPassword, deleteAppPassword } from '@/api/app-passwords'
import type { AppPassword } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import { formatDateTime } from '@/utils/date'

const loading = ref(true)
const creating = ref(false)
const passwords = ref<AppPassword[]>([])
const newLabel = ref('')
const generatedToken = ref('')
const copied = ref(false)

async function fetchPasswords() {
  loading.value = true
  try {
    const { data } = await listAppPasswords()
    passwords.value = data.app_passwords || []
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newLabel.value.trim()) return
  creating.value = true
  generatedToken.value = ''
  copied.value = false
  try {
    const { data } = await createAppPassword(newLabel.value.trim())
    generatedToken.value = data.token
    newLabel.value = ''
    await fetchPasswords()
  } catch (err: any) {
    alert(err?.response?.data?.error || 'Failed to create app password')
  } finally {
    creating.value = false
  }
}

async function handleDelete(id: string, label: string) {
  if (!confirm(`Delete app password "${label}"? Any device using it will lose access.`)) return
  try {
    await deleteAppPassword(id)
    passwords.value = passwords.value.filter(p => p.id !== id)
  } catch {
    alert('Failed to delete app password')
  }
}

async function copyToken() {
  try {
    await navigator.clipboard.writeText(generatedToken.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 2000)
  } catch {
    // fallback — the code element has select-all
  }
}

onMounted(fetchPasswords)
</script>
