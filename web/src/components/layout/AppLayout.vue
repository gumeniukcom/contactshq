<template>
  <div class="flex h-screen bg-background transition-colors duration-200">
    <Sidebar />
    <div class="flex-1 flex flex-col overflow-hidden">
      <Header />
      <main class="flex-1 overflow-y-auto p-6">
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
import { RouterView } from 'vue-router'
import Sidebar from './Sidebar.vue'
import Header from './Header.vue'
import PageSpinner from '@/components/ui/PageSpinner.vue'
</script>
