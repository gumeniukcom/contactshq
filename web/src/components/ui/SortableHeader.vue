<template>
  <th
    :class="[
      'px-4 py-3 text-left text-xs font-medium uppercase tracking-wider select-none',
      sortable ? 'cursor-pointer hover:text-accent' : '',
      isActive ? 'text-accent' : 'text-muted-foreground',
    ]"
    @click="sortable && $emit('sort', columnKey)"
  >
    <span class="inline-flex items-center gap-1">
      {{ label }}
      <svg v-if="sortable" class="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
        <path
          v-if="isActive && currentDir === 'asc'"
          d="M8 4l4 5H4l4-5z"
        />
        <path
          v-else-if="isActive && currentDir === 'desc'"
          d="M8 12L4 7h8l-4 5z"
        />
        <template v-else>
          <path d="M8 4l3 4H5l3-4z" opacity="0.3" />
          <path d="M8 12L5 8h6l-3 4z" opacity="0.3" />
        </template>
      </svg>
    </span>
  </th>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  columnKey: string
  currentSort: string
  currentDir: string
  sortable?: boolean
}>(), {
  sortable: true,
})

defineEmits<{ sort: [key: string] }>()

const isActive = computed(() => props.currentSort === props.columnKey)
</script>
