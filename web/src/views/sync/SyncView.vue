<template>
  <div class="max-w-2xl space-y-6">
    <h1 class="text-2xl font-bold text-gray-900">Sync</h1>

    <!-- Connected providers -->
    <AppCard>
      <h2 class="text-lg font-semibold text-gray-900 mb-4">Connected Providers</h2>

      <div v-if="loadingProviders" class="text-sm text-gray-400">Loading…</div>

      <div v-else-if="providers.length === 0" class="text-sm text-gray-500 py-2">
        No providers connected yet. Add a CardDAV connection below.
      </div>

      <div v-else class="space-y-3">
        <div
          v-for="p in providers"
          :key="p.id"
          class="flex items-start justify-between rounded-lg border border-gray-200 p-4"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-gray-900">{{ p.name }}</span>
              <AppBadge :color="p.connected ? 'green' : 'gray'">
                {{ p.connected ? 'Connected' : 'Disconnected' }}
              </AppBadge>
            </div>
            <p class="text-xs text-gray-500 mt-0.5 truncate">{{ p.endpoint }}</p>
            <p v-if="p.last_sync_at" class="text-xs text-gray-400 mt-0.5">
              Last sync: {{ formatDate(p.last_sync_at) }}
            </p>
            <p v-if="p.last_error" class="text-xs text-red-500 mt-0.5">{{ p.last_error }}</p>
          </div>

          <div class="flex gap-2 ml-4 shrink-0">
            <AppButton
              size="sm"
              variant="secondary"
              :loading="triggeringId === p.id"
              @click="handleTrigger(p)"
            >
              Sync Now
            </AppButton>
            <AppButton
              size="sm"
              variant="danger"
              :loading="disconnectingId === p.id"
              @click="handleDisconnect(p)"
            >
              Disconnect
            </AppButton>
          </div>
        </div>
      </div>
    </AppCard>

    <!-- Connect CardDAV -->
    <AppCard>
      <h2 class="text-lg font-semibold text-gray-900 mb-4">
        {{ hasCardDAV ? 'Update CardDAV Connection' : 'Connect CardDAV' }}
      </h2>

      <form @submit.prevent="handleConnect" class="space-y-3">
        <div>
          <label class="block text-sm font-medium text-gray-700 mb-1">Server URL</label>
          <input
            v-model="form.url"
            type="url"
            placeholder="https://dav.example.com/addressbooks/user/"
            required
            class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
          />
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Username</label>
            <input
              v-model="form.username"
              type="text"
              autocomplete="off"
              class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Password</label>
            <div class="relative">
              <input
                v-model="form.password"
                :type="showPassword ? 'text' : 'password'"
                autocomplete="off"
                class="block w-full rounded-md border border-gray-300 px-3 py-2 pr-9 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
              />
              <button
                type="button"
                class="absolute inset-y-0 right-0 flex items-center pr-2 text-gray-400 hover:text-gray-600"
                @click="showPassword = !showPassword"
              >
                <svg v-if="showPassword" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.88 9.88l-3.29-3.29m7.532 7.532l3.29 3.29M3 3l3.59 3.59m0 0A9.953 9.953 0 0112 5c4.478 0 8.268 2.943 9.543 7a10.025 10.025 0 01-4.132 4.411m0 0L21 21" />
                </svg>
                <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                </svg>
              </button>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <input
            id="skip-tls"
            v-model="form.skipTLS"
            type="checkbox"
            class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500"
          />
          <label for="skip-tls" class="text-sm text-gray-700">
            Skip TLS/SSL certificate verification
            <span class="text-xs text-gray-400">(use for self-signed certs)</span>
          </label>
        </div>

        <div class="flex items-center gap-3 pt-1">
          <AppButton type="submit" :loading="connecting">
            {{ hasCardDAV ? 'Update' : 'Connect' }}
          </AppButton>
          <span v-if="connectSuccess" class="text-sm text-green-600">Connected successfully</span>
          <span v-if="connectError" class="text-sm text-red-600">{{ connectError }}</span>
        </div>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listProviders, connectCardDAV, triggerCardDAVSync, disconnectProvider } from '@/api/sync'
import type { SyncProvider } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppBadge from '@/components/ui/AppBadge.vue'

// ── Providers list ─────────────────────────────────────────────────────────
const providers = ref<SyncProvider[]>([])
const loadingProviders = ref(true)

const hasCardDAV = computed(() => providers.value.some(p => p.provider_type === 'carddav'))

async function loadProviders() {
  loadingProviders.value = true
  try {
    const { data } = await listProviders()
    providers.value = data.providers || []
  } catch {
    providers.value = []
  } finally {
    loadingProviders.value = false
  }
}

// ── Trigger ────────────────────────────────────────────────────────────────
const triggeringId = ref<string | null>(null)

async function handleTrigger(p: SyncProvider) {
  triggeringId.value = p.id
  try {
    await triggerCardDAVSync()
  } finally {
    triggeringId.value = null
  }
}

// ── Disconnect ─────────────────────────────────────────────────────────────
const disconnectingId = ref<string | null>(null)

async function handleDisconnect(p: SyncProvider) {
  disconnectingId.value = p.id
  try {
    await disconnectProvider(p.id)
    await loadProviders()
  } finally {
    disconnectingId.value = null
  }
}

// ── Connect form ───────────────────────────────────────────────────────────
const form = ref({ url: '', username: '', password: '', skipTLS: false })
const showPassword = ref(false)
const connecting = ref(false)
const connectSuccess = ref(false)
const connectError = ref('')

async function handleConnect() {
  connecting.value = true
  connectSuccess.value = false
  connectError.value = ''
  try {
    await connectCardDAV({ url: form.value.url, username: form.value.username, password: form.value.password, skip_tls_verify: form.value.skipTLS })
    connectSuccess.value = true
    form.value = { url: '', username: '', password: '', skipTLS: false }
    showPassword.value = false
    setTimeout(() => { connectSuccess.value = false }, 3000)
    await loadProviders()
  } catch (e: unknown) {
    connectError.value = (e as Error)?.message ?? 'Connection failed'
  } finally {
    connecting.value = false
  }
}

// ── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(loadProviders)

// ── Helpers ────────────────────────────────────────────────────────────────
function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}
</script>
