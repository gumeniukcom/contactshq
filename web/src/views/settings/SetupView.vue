<template>
  <div class="max-w-3xl">
    <h1 class="text-2xl font-bold text-foreground mb-2">Connect Your Devices</h1>
    <p class="text-muted-foreground mb-6">
      Sync your contacts with phones, tablets, and desktop apps using CardDAV.
    </p>

    <!-- CardDAV URL reference -->
    <AppCard class="mb-6">
      <h2 class="text-lg font-semibold text-foreground mb-3">Your CardDAV URL</h2>
      <div class="flex items-center gap-2">
        <code class="flex-1 px-3 py-2 bg-muted border border-border rounded font-mono text-sm text-foreground break-all select-all">{{ davUrl }}</code>
        <AppButton variant="secondary" size="sm" @click="copyUrl(davUrl)">
          {{ copiedUrl ? 'Copied!' : 'Copy' }}
        </AppButton>
      </div>
      <p class="mt-2 text-sm text-muted-foreground">
        Use an <RouterLink to="/settings/app-passwords" class="text-accent underline">App Password</RouterLink> instead of your main password for CardDAV clients.
      </p>
    </AppCard>

    <!-- iPhone/iPad -->
    <AppCard class="mb-6">
      <h2 class="text-lg font-semibold text-foreground mb-4">iPhone / iPad</h2>

      <div class="space-y-4">
        <!-- Option A: Profile -->
        <div class="p-4 bg-blue-50 dark:bg-blue-500/20 rounded-md">
          <p class="font-medium text-blue-900 dark:text-blue-300 mb-2">Option A: One-Tap Setup (Recommended)</p>
          <p class="text-sm text-blue-800 dark:text-blue-400 mb-3">
            Download a configuration profile that sets up CardDAV automatically.
          </p>
          <AppButton variant="primary" size="sm" @click="downloadProfile">
            Download Profile
          </AppButton>
        </div>

        <!-- Option B: Manual -->
        <div>
          <p class="font-medium text-foreground mb-2">Option B: Manual Setup</p>
          <ol class="list-decimal list-inside text-sm text-muted-foreground space-y-1">
            <li>Open <strong>Settings</strong> → <strong>Apps</strong> → <strong>Contacts</strong></li>
            <li>Tap <strong>Contacts Accounts</strong> → <strong>Add Account</strong></li>
            <li>Select <strong>Other</strong> → <strong>Add CardDAV Account</strong></li>
            <li>Enter:
              <ul class="ml-6 mt-1 space-y-1 list-disc">
                <li>Server: <code class="bg-muted px-1 rounded text-foreground">{{ hostname }}</code></li>
                <li>User Name: <code class="bg-muted px-1 rounded text-foreground">{{ userEmail }}</code></li>
                <li>Password: your <RouterLink to="/settings/app-passwords" class="text-accent underline">app password</RouterLink></li>
              </ul>
            </li>
            <li>Tap <strong>Next</strong> — contacts will start syncing</li>
          </ol>
        </div>
      </div>
    </AppCard>

    <!-- macOS -->
    <AppCard class="mb-6">
      <h2 class="text-lg font-semibold text-foreground mb-4">macOS</h2>
      <ol class="list-decimal list-inside text-sm text-muted-foreground space-y-1">
        <li>Open <strong>System Settings</strong> → <strong>Internet Accounts</strong></li>
        <li>Click <strong>Add Other Account</strong> → <strong>CardDAV account</strong></li>
        <li>Select <strong>Manual</strong> account type</li>
        <li>Enter:
          <ul class="ml-6 mt-1 space-y-1 list-disc">
            <li>Server: <code class="bg-muted px-1 rounded text-foreground">{{ hostname }}</code></li>
            <li>User Name: <code class="bg-muted px-1 rounded text-foreground">{{ userEmail }}</code></li>
            <li>Password: your <RouterLink to="/settings/app-passwords" class="text-accent underline">app password</RouterLink></li>
          </ul>
        </li>
        <li>Click <strong>Sign In</strong></li>
      </ol>
    </AppCard>

    <!-- Thunderbird -->
    <AppCard class="mb-6">
      <h2 class="text-lg font-semibold text-foreground mb-4">Thunderbird</h2>
      <ol class="list-decimal list-inside text-sm text-muted-foreground space-y-1">
        <li>Install the <strong>CardBook</strong> add-on from Thunderbird Add-ons</li>
        <li>Open CardBook → <strong>Address Book</strong> → <strong>New Address Book</strong></li>
        <li>Select <strong>Remote</strong> → <strong>CardDAV</strong></li>
        <li>Enter URL: <code class="bg-muted px-1 rounded text-foreground break-all">{{ davUrl }}</code></li>
        <li>Enter your email and <RouterLink to="/settings/app-passwords" class="text-accent underline">app password</RouterLink></li>
        <li>Click <strong>Validate</strong>, then <strong>Next</strong></li>
      </ol>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'

const auth = useAuthStore()
const copiedUrl = ref(false)

const userEmail = computed(() => auth.user?.email || '')
const hostname = computed(() => window.location.hostname)
const davUrl = computed(() => {
  const origin = window.location.origin
  return `${origin}/dav/${userEmail.value}/contacts/`
})

async function copyUrl(url: string) {
  try {
    await navigator.clipboard.writeText(url)
    copiedUrl.value = true
    setTimeout(() => { copiedUrl.value = false }, 2000)
  } catch { /* noop */ }
}

function downloadProfile() {
  const token = localStorage.getItem('access_token')
  // Use fetch to get the profile with auth, then trigger download
  fetch('/api/v1/setup/ios-profile', {
    headers: { Authorization: `Bearer ${token}` },
  })
    .then(r => r.blob())
    .then(blob => {
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = 'ContactsHQ.mobileconfig'
      a.click()
      URL.revokeObjectURL(url)
    })
    .catch(() => alert('Failed to download profile'))
}
</script>
