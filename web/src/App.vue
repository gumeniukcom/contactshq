<template>
  <RouteProgress />
  <RouterView />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import RouteProgress from '@/components/ui/RouteProgress.vue'
</script>
