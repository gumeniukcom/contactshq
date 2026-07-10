<template>
  <RouteProgress />
  <RouterView />
  <AppToast />
</template>

<script setup lang="ts">
import { RouterView } from 'vue-router'
import RouteProgress from '@/components/ui/RouteProgress.vue'
import AppToast from '@/components/ui/AppToast.vue'
</script>
