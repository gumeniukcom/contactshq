<template>
  <div class="flex h-screen bg-background transition-colors duration-200">
    <Sidebar :open="sidebarOpen" @close="sidebarOpen = false" />
    <div class="flex-1 flex flex-col overflow-hidden min-w-0">
      <Header @toggle-sidebar="sidebarOpen = !sidebarOpen" />
      <main class="flex-1 overflow-y-auto p-4 sm:p-6">
        <RouterView v-slot="{ Component }">
          <Suspense>
            <component :is="Component" />
            <template #fallback>
              <PageSpinner />
            </template>
          </Suspense>
        </RouterView>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { RouterView } from 'vue-router'
import Sidebar from './Sidebar.vue'
import Header from './Header.vue'
import PageSpinner from '@/components/ui/PageSpinner.vue'

const sidebarOpen = ref(false)
</script>
