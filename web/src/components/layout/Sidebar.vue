<template>
  <aside class="w-64 bg-sidebar dark:backdrop-blur-xl border-r border-sidebar-border flex flex-col">
    <div class="h-16 flex items-center px-6 border-b border-sidebar-border">
      <RouterLink to="/" class="flex items-center gap-2">
        <svg class="h-7 w-7 text-accent" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2"
            d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0z" />
        </svg>
        <span class="text-lg font-bold text-foreground">ContactsHQ</span>
      </RouterLink>
    </div>

    <nav class="flex-1 px-4 py-4 space-y-1 overflow-y-auto">
      <NavLink to="/" icon="home" exact>Dashboard</NavLink>
      <NavLink to="/contacts" icon="users">Contacts</NavLink>
      <NavLink to="/contacts/duplicates" icon="duplicate">
        Duplicates
        <span v-if="pendingDuplicates > 0"
          class="ml-auto inline-flex items-center justify-center h-4 min-w-4 px-1 rounded-full text-xs font-bold bg-orange-500 text-white">
          {{ pendingDuplicates }}
        </span>
      </NavLink>
      <NavLink to="/import" icon="upload">Import</NavLink>
      <NavLink to="/export" icon="download">Export</NavLink>
      <NavLink to="/pipelines" icon="pipeline">Pipelines</NavLink>
      <NavLink to="/backup" icon="backup">Backup</NavLink>
      <NavLink to="/credentials" icon="key">Credentials</NavLink>
      <NavLink to="/sync/conflicts" icon="conflict">
        Conflicts
        <span v-if="pendingConflicts > 0"
          class="ml-auto inline-flex items-center justify-center h-4 min-w-4 px-1 rounded-full text-xs font-bold bg-red-500 text-white">
          {{ pendingConflicts }}
        </span>
      </NavLink>

      <div class="pt-4 mt-4 border-t border-sidebar-border">
        <p class="px-3 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Settings</p>
        <NavLink to="/settings/setup" icon="phone">Connect Devices</NavLink>
        <NavLink to="/settings/profile" icon="user">Profile</NavLink>
        <NavLink to="/settings/password" icon="lock">Password</NavLink>
        <NavLink to="/settings/app-passwords" icon="key-alt">App Passwords</NavLink>
        <NavLink to="/settings/google" icon="cloud">Google</NavLink>
      </div>

      <div v-if="auth.isAdmin" class="pt-4 mt-4 border-t border-sidebar-border">
        <p class="px-3 text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">Admin</p>
        <NavLink to="/admin/users" icon="shield">Users</NavLink>
      </div>
    </nav>
  </aside>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import NavLink from './NavLink.vue'
import { countConflicts } from '@/api/sync'
import { countDuplicates } from '@/api/contacts'

const auth = useAuthStore()
const pendingConflicts = ref(0)
const pendingDuplicates = ref(0)

async function fetchCounts() {
  await Promise.allSettled([
    countConflicts().then(({ data }) => { pendingConflicts.value = data.pending }),
    countDuplicates().then(({ data }) => { pendingDuplicates.value = data.pending }),
  ])
}

onMounted(fetchCounts)
</script>
