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
    purple: 'bg-purple-100 text-purple-800',
    blue: 'bg-blue-100 text-blue-800',
    green: 'bg-green-100 text-green-800',
    red: 'bg-red-100 text-red-800',
    gray: 'bg-gray-100 text-gray-800',
  }
  return map[props.color]
})
</script>
