<template>
  <div class="flex flex-col items-center justify-center py-20 text-center">
    <p class="text-6xl font-bold text-muted-foreground">404</p>
    <h1 class="mt-4 text-xl font-semibold text-foreground">Page not found</h1>
    <p class="mt-2 text-sm text-muted-foreground">
      The page you are looking for does not exist or has moved.
    </p>
    <RouterLink
      to="/"
      class="mt-6 inline-flex items-center rounded-md bg-accent px-4 py-2 text-sm font-medium text-accent-foreground hover:bg-accent/90"
    >
      Back to Dashboard
    </RouterLink>
  </div>
</template>

<script setup lang="ts">
import { RouterLink } from 'vue-router'
</script>
