<template>
  <header class="h-16 bg-white border-b border-gray-200 flex items-center justify-end px-6">
    <div class="flex items-center gap-4">
      <span class="text-sm text-gray-700">{{ auth.user?.display_name || auth.user?.email }}</span>
      <button
        class="text-sm text-gray-500 hover:text-gray-700 font-medium"
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

const router = useRouter()
const auth = useAuthStore()

function handleLogout() {
  auth.logout()
  router.push({ name: 'login' })
}
</script>
