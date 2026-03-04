<template>
  <div
    v-if="selectedCount > 0"
    class="fixed bottom-4 left-1/2 -translate-x-1/2 z-50 bg-gray-900 text-white rounded-xl shadow-2xl px-6 py-3 flex items-center gap-4"
  >
    <span class="text-sm font-medium">
      {{ selectedCount }} selected
    </span>
    <button
      v-if="selectedCount < totalCount"
      class="text-sm text-indigo-300 hover:text-indigo-200"
      @click="$emit('selectAll')"
    >
      Select all {{ totalCount }}
    </button>

    <div class="h-5 w-px bg-gray-600" />

    <button
      class="inline-flex items-center gap-1.5 text-sm hover:text-indigo-300 transition-colors"
      @click="$emit('export')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
      </svg>
      Export
    </button>

    <button
      class="inline-flex items-center gap-1.5 text-sm text-red-400 hover:text-red-300 transition-colors"
      @click="$emit('delete')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
      </svg>
      Delete
    </button>

    <button
      class="ml-2 text-gray-400 hover:text-white"
      @click="$emit('close')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  selectedCount: number
  totalCount: number
}>()

defineEmits<{
  selectAll: []
  export: []
  delete: []
  close: []
}>()
</script>
