<template>
  <Teleport to="body">
    <div class="fixed top-4 right-4 z-50 space-y-2">
      <TransitionGroup name="toast">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="[
            'flex items-center gap-2 rounded-lg px-4 py-3 shadow-lg text-sm min-w-[300px]',
            toast.type === 'success' ? 'bg-green-600 text-white' : '',
            toast.type === 'error' ? 'bg-red-600 text-white' : '',
            toast.type === 'info' ? 'bg-indigo-600 text-white' : '',
          ]"
        >
          <span class="flex-1">{{ toast.message }}</span>
          <button class="text-white/80 hover:text-white" @click="remove(toast.id)">
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from 'vue'

interface Toast {
  id: number
  message: string
  type: 'success' | 'error' | 'info'
}

const toasts = ref<Toast[]>([])
let nextId = 0

function add(message: string, type: Toast['type'] = 'info') {
  const id = nextId++
  toasts.value.push({ id, message, type })
  setTimeout(() => remove(id), 4000)
}

function remove(id: number) {
  toasts.value = toasts.value.filter((t) => t.id !== id)
}

defineExpose({ add })
</script>

<style scoped>
.toast-enter-active,
.toast-leave-active {
  transition: all 0.3s ease;
}
.toast-enter-from {
  opacity: 0;
  transform: translateX(30px);
}
.toast-leave-to {
  opacity: 0;
  transform: translateX(30px);
}
</style>
