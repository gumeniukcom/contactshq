<template>
  <div v-if="totalPages > 1 || showPerPage" class="flex items-center justify-between px-4 py-3">
    <div class="flex items-center gap-3">
      <span class="text-sm text-gray-700">
        Showing {{ (page - 1) * perPage + 1 }} to {{ Math.min(page * perPage, total) }} of {{ total }}
      </span>
      <select
        v-if="showPerPage"
        :value="perPage"
        class="text-sm border border-gray-300 rounded px-2 py-1 bg-white"
        @change="$emit('update:perPage', Number(($event.target as HTMLSelectElement).value))"
      >
        <option :value="20">20 / page</option>
        <option :value="50">50 / page</option>
        <option :value="100">100 / page</option>
      </select>
    </div>
    <div v-if="totalPages > 1" class="flex gap-1">
      <button
        :disabled="page <= 1"
        class="px-3 py-1 text-sm rounded border border-gray-300 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
        @click="$emit('update:page', page - 1)"
      >
        Previous
      </button>
      <button
        v-for="p in displayPages"
        :key="p"
        :class="[
          'px-3 py-1 text-sm rounded border',
          p === page ? 'bg-indigo-600 text-white border-indigo-600' : 'border-gray-300 hover:bg-gray-50',
        ]"
        @click="$emit('update:page', p)"
      >
        {{ p }}
      </button>
      <button
        :disabled="page >= totalPages"
        class="px-3 py-1 text-sm rounded border border-gray-300 hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
        @click="$emit('update:page', page + 1)"
      >
        Next
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  page: number
  perPage: number
  total: number
  showPerPage?: boolean
}>(), {
  showPerPage: false,
})

defineEmits<{
  'update:page': [value: number]
  'update:perPage': [value: number]
}>()

const totalPages = computed(() => Math.ceil(props.total / props.perPage))

const displayPages = computed(() => {
  const pages: number[] = []
  const start = Math.max(1, props.page - 2)
  const end = Math.min(totalPages.value, props.page + 2)
  for (let i = start; i <= end; i++) pages.push(i)
  return pages
})
</script>
