<template>
  <div class="bg-white rounded-lg shadow border border-gray-200 p-6">
    <h3 v-if="title" class="text-lg font-medium text-gray-900 mb-4">{{ title }}</h3>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ title?: string }>()
</script>
