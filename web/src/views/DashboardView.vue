<template>
  <div>
    <h1 class="text-2xl font-bold text-foreground mb-6">
      Welcome back, {{ auth.user?.display_name || 'User' }}
    </h1>

    <div class="grid grid-cols-1 md:grid-cols-3 gap-6 mb-8">
      <AppCard>
        <div class="flex items-center gap-4">
          <div class="h-12 w-12 rounded-lg bg-accent/10 flex items-center justify-center">
            <svg class="h-6 w-6 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z" />
            </svg>
          </div>
          <div>
            <p class="text-sm text-muted-foreground">Total Contacts</p>
            <p class="text-2xl font-bold text-foreground">{{ totalContacts }}</p>
          </div>
        </div>
      </AppCard>

      <AppCard>
        <div class="flex items-center gap-4">
          <div class="h-12 w-12 rounded-lg bg-green-100 dark:bg-green-500/20 flex items-center justify-center">
            <svg class="h-6 w-6 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
            </svg>
          </div>
          <div>
            <p class="text-sm text-muted-foreground">CardDAV</p>
            <p class="text-2xl font-bold text-foreground">Active</p>
          </div>
        </div>
      </AppCard>

      <AppCard>
        <div class="flex items-center gap-4">
          <div class="h-12 w-12 rounded-lg bg-purple-100 dark:bg-purple-500/20 flex items-center justify-center">
            <svg class="h-6 w-6 text-purple-600 dark:text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
                d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z" />
            </svg>
          </div>
          <div>
            <p class="text-sm text-muted-foreground">Role</p>
            <p class="text-2xl font-bold text-foreground capitalize">{{ auth.user?.role }}</p>
          </div>
        </div>
      </AppCard>
    </div>

    <h2 class="text-lg font-semibold text-foreground mb-4">Quick Actions</h2>
    <div class="flex flex-wrap gap-3">
      <RouterLink to="/contacts/new">
        <AppButton>Add Contact</AppButton>
      </RouterLink>
      <RouterLink to="/import">
        <AppButton variant="secondary">Import Contacts</AppButton>
      </RouterLink>
      <RouterLink to="/export">
        <AppButton variant="secondary">Export Contacts</AppButton>
      </RouterLink>
      <RouterLink to="/backup">
        <AppButton variant="secondary">Create Backup</AppButton>
      </RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { listContacts } from '@/api/contacts'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'

const auth = useAuthStore()
const totalContacts = ref(0)

onMounted(async () => {
  try {
    const { data } = await listContacts({ limit: 1 })
    totalContacts.value = data.total
  } catch {
    // ignore
  }
})
</script>
