<template>
  <Teleport to="body">
    <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center">
      <div class="fixed inset-0 bg-black/50" aria-hidden="true" @click="emit('close')" />
      <div
        ref="dialog"
        role="dialog"
        aria-modal="true"
        :aria-label="label"
        tabindex="-1"
        class="relative bg-card border border-border rounded-lg shadow-xl max-w-lg w-full mx-4 p-6 max-h-[90vh] overflow-y-auto"
      >
        <button
          class="absolute top-4 right-4 text-muted-foreground hover:text-foreground"
          aria-label="Close dialog"
          @click="emit('close')"
        >
          <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
        <slot />
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, toRef } from 'vue'
import { useFocusTrap } from '@/composables/useFocusTrap'

const props = withDefaults(defineProps<{ show: boolean; label?: string }>(), { label: 'Dialog' })
const emit = defineEmits<{ close: [] }>()

const dialog = ref<HTMLElement | null>(null)
useFocusTrap(dialog, toRef(props, 'show'), () => emit('close'))
</script>
