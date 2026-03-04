<template>
  <Teleport to="body">
    <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center">
      <div class="fixed inset-0 bg-black/50" @click="$emit('close')" />
      <div class="relative bg-card border border-border rounded-lg shadow-xl max-w-lg w-full mx-4 p-6">
        <button
          class="absolute top-4 right-4 text-muted-foreground hover:text-foreground"
          @click="$emit('close')"
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
defineProps<{ show: boolean }>()
defineEmits<{ close: [] }>()
</script>
