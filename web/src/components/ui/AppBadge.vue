<template>
  <span
    :class="[
      'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
      colorClasses,
    ]"
  >
    <slot />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    color?: 'purple' | 'blue' | 'green' | 'red' | 'gray'
  }>(),
  { color: 'gray' },
)

const colorClasses = computed(() => {
  const map = {
    purple: 'bg-purple-100 text-purple-800 dark:bg-purple-500/20 dark:text-purple-300',
    blue: 'bg-blue-100 text-blue-800 dark:bg-blue-500/20 dark:text-blue-300',
    green: 'bg-green-100 text-green-800 dark:bg-green-500/20 dark:text-green-300',
    red: 'bg-red-100 text-red-800 dark:bg-red-500/20 dark:text-red-300',
    gray: 'bg-muted text-muted-foreground',
  }
  return map[props.color]
})
</script>
