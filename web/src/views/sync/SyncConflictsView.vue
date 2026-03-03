<template>
  <div class="max-w-3xl">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-gray-900">
        Sync Conflicts
        <span v-if="total > 0" class="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-red-100 text-red-800">
          {{ total }}
        </span>
      </h1>
      <div class="flex gap-2">
        <select v-model="statusFilter" class="text-sm rounded-md border-gray-300 border px-3 py-2">
          <option value="">All</option>
          <option value="pending">Pending</option>
          <option value="resolved">Resolved</option>
          <option value="dismissed">Dismissed</option>
        </select>
      </div>
    </div>

    <div v-if="loading" class="py-8 text-center text-gray-400">Loading…</div>
    <div v-else-if="conflicts.length === 0" class="py-12 text-center text-gray-400">
      <svg class="mx-auto h-12 w-12 text-gray-300 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
          d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <p>No conflicts found.</p>
    </div>

    <div v-else class="space-y-3">
      <div
        v-for="c in conflicts"
        :key="c.id"
        class="bg-white rounded-lg border border-gray-200 p-4 flex items-start justify-between gap-4"
      >
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2 mb-1">
            <span :class="statusClass(c.status)" class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium">
              {{ c.status }}
            </span>
            <span class="text-xs text-gray-400">{{ c.provider_type }}</span>
          </div>
          <p class="text-sm text-gray-700 font-mono truncate">{{ c.remote_id }}</p>
          <p class="text-xs text-gray-400 mt-1">{{ formatDate(c.created_at) }}</p>
          <div v-if="parsedDiffs(c).length > 0" class="mt-2 flex flex-wrap gap-1">
            <span
              v-for="d in parsedDiffs(c)"
              :key="d.field"
              class="inline-flex items-center px-1.5 py-0.5 rounded text-xs bg-amber-50 text-amber-700 border border-amber-200"
            >
              {{ d.field }}
            </span>
          </div>
        </div>
        <div class="flex flex-col gap-2 shrink-0">
          <RouterLink
            v-if="c.status === 'pending'"
            :to="{ name: 'sync-conflict-detail', params: { id: c.id } }"
            class="text-xs font-medium text-indigo-600 hover:text-indigo-800 whitespace-nowrap"
          >
            Resolve →
          </RouterLink>
          <button
            v-if="c.status === 'pending'"
            class="text-xs text-gray-400 hover:text-gray-600"
            @click="dismiss(c.id)"
          >
            Dismiss
          </button>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="total > limit" class="mt-6 flex justify-center gap-2">
      <AppButton size="sm" variant="secondary" :disabled="offset === 0" @click="prevPage">Previous</AppButton>
      <span class="text-sm text-gray-500 self-center">{{ page + 1 }} / {{ Math.ceil(total / limit) }}</span>
      <AppButton size="sm" variant="secondary" :disabled="offset + limit >= total" @click="nextPage">Next</AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { listConflicts, dismissConflict } from '@/api/sync'
import type { SyncConflict, FieldDiff } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'

const loading = ref(false)
const conflicts = ref<SyncConflict[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const page = ref(0)
const statusFilter = ref('pending')

async function fetchConflicts() {
  loading.value = true
  try {
    const { data } = await listConflicts({ status: statusFilter.value || undefined, limit, offset: offset.value })
    conflicts.value = data.conflicts
    total.value = data.total
  } finally {
    loading.value = false
  }
}

watch(statusFilter, () => {
  offset.value = 0
  page.value = 0
  fetchConflicts()
})

function prevPage() {
  offset.value = Math.max(0, offset.value - limit)
  page.value--
  fetchConflicts()
}

function nextPage() {
  offset.value += limit
  page.value++
  fetchConflicts()
}

async function dismiss(id: string) {
  await dismissConflict(id)
  fetchConflicts()
}

function parsedDiffs(c: SyncConflict): FieldDiff[] {
  try {
    return JSON.parse(c.field_diffs) as FieldDiff[]
  } catch {
    return []
  }
}

function statusClass(status: string) {
  if (status === 'pending') return 'bg-yellow-100 text-yellow-800'
  if (status === 'resolved') return 'bg-green-100 text-green-800'
  return 'bg-gray-100 text-gray-600'
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString()
}

onMounted(fetchConflicts)
</script>
