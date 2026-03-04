<template>
  <header class="h-16 bg-card border-b border-border flex items-center justify-end px-6">
    <div class="flex items-center gap-4">
      <!-- Theme toggle -->
      <button
        class="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        :title="`Theme: ${theme}`"
        @click="toggleTheme"
      >
        <!-- Sun (light) -->
        <svg v-if="resolved === 'light'" class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 3v1m0 16v1m9-9h-1M4 12H3m15.364 6.364l-.707-.707M6.343 6.343l-.707-.707m12.728 0l-.707.707M6.343 17.657l-.707.707M16 12a4 4 0 11-8 0 4 4 0 018 0z" />
        </svg>
        <!-- Moon (dark) -->
        <svg v-else class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
        </svg>
      </button>

      <span class="text-sm text-muted-foreground">{{ auth.user?.display_name || auth.user?.email }}</span>
      <button
        class="text-sm text-muted-foreground hover:text-foreground font-medium"
        @click="handleLogout"
      >
        Logout
      </button>
    </div>
  </header>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useTheme } from '@/composables/useTheme'

const router = useRouter()
const auth = useAuthStore()
const { theme, resolved, toggleTheme } = useTheme()

function handleLogout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>
