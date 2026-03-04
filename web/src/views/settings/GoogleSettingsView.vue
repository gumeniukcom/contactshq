<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-gray-900 mb-6">Google Contacts</h1>

    <!-- Loading -->
    <div v-if="loading" class="text-gray-500">Loading...</div>

    <!-- Connected state -->
    <AppCard v-else-if="status?.connected">
      <div class="flex items-center gap-3 mb-4">
        <div class="h-10 w-10 rounded-full bg-green-100 flex items-center justify-center">
          <svg class="h-5 w-5 text-green-600" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <div>
          <p class="font-medium text-gray-900">Connected to Google Contacts</p>
          <p v-if="status.last_sync_at" class="text-sm text-gray-500">
            Last synced: {{ new Date(status.last_sync_at).toLocaleString() }}
          </p>
        </div>
      </div>

      <p v-if="status.last_error" class="text-sm text-red-600 mb-4">
        Last error: {{ status.last_error }}
      </p>

      <p class="text-sm text-gray-600 mb-4">
        To sync contacts, create a Pipeline with Google as the source or destination.
      </p>

      <div class="flex gap-3">
        <RouterLink to="/pipelines/new">
          <AppButton variant="secondary" size="sm">Create Pipeline</AppButton>
        </RouterLink>
        <AppButton variant="danger" size="sm" :loading="disconnecting" @click="handleDisconnect">
          Disconnect
        </AppButton>
      </div>
    </AppCard>

    <!-- Not connected state -->
    <template v-else>
      <!-- URL params feedback -->
      <div v-if="connectedParam" class="mb-4 p-3 rounded-md bg-green-50 text-green-800 text-sm">
        Successfully connected to Google Contacts!
      </div>
      <div v-if="errorParam" class="mb-4 p-3 rounded-md bg-red-50 text-red-800 text-sm">
        Connection failed: {{ errorParam }}
      </div>

      <AppCard>
        <h2 class="text-lg font-semibold text-gray-900 mb-4">Connect Google Contacts</h2>

        <div class="mb-6 p-4 bg-blue-50 rounded-md text-sm text-blue-900 space-y-2">
          <p class="font-medium">Setup instructions:</p>
          <ol class="list-decimal list-inside space-y-1">
            <li>Go to <a href="https://console.cloud.google.com/" target="_blank" class="underline">Google Cloud Console</a> and create a project</li>
            <li>Enable the <strong>People API</strong> (APIs & Services > Library)</li>
            <li>Configure <strong>OAuth Consent Screen</strong> > External > publish to Production</li>
            <li>Add scope: <code class="bg-blue-100 px-1 rounded">https://www.googleapis.com/auth/contacts</code></li>
            <li>Create <strong>OAuth 2.0 Client ID</strong> > Web application</li>
            <li>
              Add redirect URI:
              <code class="bg-blue-100 px-1 rounded break-all">{{ redirectUri }}</code>
            </li>
            <li>Copy Client ID and Secret below</li>
          </ol>
        </div>

        <form class="space-y-4" @submit.prevent="handleConnect">
          <AppInput
            v-model="form.client_id"
            label="Client ID"
            placeholder="123456789-abc.apps.googleusercontent.com"
            id="google-client-id"
          />
          <AppInput
            v-model="form.client_secret"
            label="Client Secret"
            type="password"
            placeholder="GOCSPX-..."
            id="google-client-secret"
          />

          <div class="flex justify-end">
            <AppButton type="submit" :loading="connecting" :disabled="!form.client_id || !form.client_secret">
              Connect with Google
            </AppButton>
          </div>
        </form>
      </AppCard>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { getGoogleStatus, initGoogleAuth, disconnectGoogle } from '@/api/google'
import type { GoogleStatus } from '@/api/google'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'

const route = useRoute()
const loading = ref(true)
const connecting = ref(false)
const disconnecting = ref(false)
const status = ref<GoogleStatus | null>(null)

const connectedParam = computed(() => route.query.connected === 'true')
const errorParam = computed(() => route.query.error as string | undefined)
const redirectUri = computed(() => `${window.location.origin}/api/v1/auth/google/callback`)

const form = reactive({
  client_id: '',
  client_secret: '',
})

async function fetchStatus() {
  loading.value = true
  try {
    const { data } = await getGoogleStatus()
    status.value = data
  } catch {
    status.value = { connected: false }
  } finally {
    loading.value = false
  }
}

async function handleConnect() {
  connecting.value = true
  try {
    const { data } = await initGoogleAuth({
      client_id: form.client_id,
      client_secret: form.client_secret,
    })
    window.location.href = data.auth_url
  } catch (err: any) {
    alert(err?.response?.data?.error || 'Failed to start Google auth')
  } finally {
    connecting.value = false
  }
}

async function handleDisconnect() {
  if (!confirm('Disconnect Google Contacts? This will revoke access and remove saved tokens.')) return
  disconnecting.value = true
  try {
    await disconnectGoogle()
    status.value = { connected: false }
  } catch {
    alert('Failed to disconnect')
  } finally {
    disconnecting.value = false
  }
}

onMounted(fetchStatus)
</script>
