<template>
  <div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-gray-200">
      <thead class="bg-gray-50">
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            class="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody class="bg-white divide-y divide-gray-200">
        <tr v-if="loading">
          <td :colspan="columns.length" class="px-6 py-8 text-center text-gray-500">
            Loading...
          </td>
        </tr>
        <tr v-else-if="!rows.length">
          <td :colspan="columns.length" class="px-6 py-8 text-center text-gray-500">
            {{ emptyText }}
          </td>
        </tr>
        <slot v-else name="body" :rows="rows" />
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    columns: Array<{ key: string; label: string }>
    rows: unknown[]
    loading?: boolean
    emptyText?: string
  }>(),
  {
    loading: false,
    emptyText: 'No data found',
  },
)
</script>
