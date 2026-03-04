<template>
  <div class="overflow-x-auto">
    <table class="min-w-full divide-y divide-border">
      <thead class="bg-muted/50">
        <tr>
          <th
            v-for="col in columns"
            :key="col.key"
            class="px-4 py-3 text-left text-xs font-medium text-muted-foreground uppercase tracking-wider"
          >
            {{ col.label }}
          </th>
        </tr>
      </thead>
      <tbody class="bg-card divide-y divide-border">
        <tr v-if="loading">
          <td :colspan="columns.length" class="px-4 py-8 text-center text-muted-foreground">
            Loading...
          </td>
        </tr>
        <tr v-else-if="!rows.length">
          <td :colspan="columns.length" class="px-4 py-8 text-center text-muted-foreground">
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
