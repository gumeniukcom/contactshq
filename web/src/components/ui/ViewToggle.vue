<template>
  <div class="inline-flex rounded-lg border border-gray-300 bg-white">
    <button
      class="p-2 rounded-l-lg transition-colors"
      :class="modelValue === 'table' ? 'bg-indigo-50 text-indigo-600' : 'text-gray-400 hover:text-gray-600'"
      title="Table view"
      @click="$emit('update:modelValue', 'table')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
      </svg>
    </button>
    <button
      class="p-2 rounded-r-lg border-l border-gray-300 transition-colors"
      :class="modelValue === 'card' ? 'bg-indigo-50 text-indigo-600' : 'text-gray-400 hover:text-gray-600'"
      title="Card view"
      @click="$emit('update:modelValue', 'card')"
    >
      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
        <path stroke-linecap="round" stroke-linejoin="round" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" />
      </svg>
    </button>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  modelValue: 'table' | 'card'
}>()

defineEmits<{
  'update:modelValue': [value: 'table' | 'card']
}>()
</script>
