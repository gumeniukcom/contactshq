<template>
  <div class="bg-card rounded-lg border border-border p-4">
    <h3 v-if="title" class="text-lg font-medium text-card-foreground mb-4">{{ title }}</h3>
    <slot />
  </div>
</template>

<script setup lang="ts">
defineProps<{ title?: string }>()
</script>
