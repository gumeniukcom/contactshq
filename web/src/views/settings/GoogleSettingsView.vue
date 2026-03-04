<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-foreground mb-6">Google Contacts</h1>

    <!-- Loading -->
    <div v-if="loading" class="text-muted-foreground">Loading...</div>

    <!-- Connected state -->
    <AppCard v-else-if="status?.connected">
      <div class="flex items-center gap-3 mb-4">
        <div class="h-10 w-10 rounded-full bg-green-100 dark:bg-green-500/20 flex items-center justify-center">
          <svg class="h-5 w-5 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <div>
          <p class="font-medium text-foreground">Connected to Google Contacts</p>
          <p v-if="status.last_sync_at" class="text-sm text-muted-foreground">
            Last synced: {{ formatDateTime(status.last_sync_at) }}
          </p>
        </div>
      </div>

      <p v-if="status.last_error" class="text-sm text-destructive mb-4">
        Last error: {{ status.last_error }}
      </p>

      <p class="text-sm text-muted-foreground mb-4">
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
      <div v-if="connectedParam" class="mb-4 p-3 rounded-md bg-green-50 dark:bg-green-500/20 text-green-800 dark:text-green-300 text-sm">
        Successfully connected to Google Contacts!
      </div>
      <div v-if="errorParam" class="mb-4 p-3 rounded-md bg-red-50 dark:bg-red-500/20 text-red-800 dark:text-red-300 text-sm">
        Connection failed: {{ errorParam }}
      </div>

      <AppCard>
        <h2 class="text-lg font-semibold text-foreground mb-4">Connect Google Contacts</h2>

        <div class="mb-6 p-4 bg-blue-50 dark:bg-blue-500/20 rounded-md text-sm text-blue-900 dark:text-blue-300 space-y-2">
          <p class="font-medium">Setup instructions:</p>
          <ol class="list-decimal list-inside space-y-1">
            <li>Go to <a href="https://console.cloud.google.com/" target="_blank" class="underline">Google Cloud Console</a> and create a project</li>
            <li>Enable the <strong>People API</strong> (APIs & Services > Library)</li>
            <li>Configure <strong>OAuth Consent Screen</strong> > External > publish to Production</li>
            <li>Add scope: <code class="bg-blue-100 dark:bg-blue-500/30 px-1 rounded">https://www.googleapis.com/auth/contacts</code></li>
            <li>Create <strong>OAuth 2.0 Client ID</strong> > Web application</li>
            <li>
              Add redirect URI:
              <code class="bg-blue-100 dark:bg-blue-500/30 px-1 rounded break-all">{{ redirectUri }}</code>
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
import { formatDateTime } from '@/utils/date'

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
