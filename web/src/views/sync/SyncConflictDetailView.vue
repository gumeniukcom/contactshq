<template>
  <div class="max-w-3xl">
    <div class="flex items-center gap-3 mb-6">
      <button @click="router.back()" class="text-gray-400 hover:text-gray-600">
        <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
      </button>
      <h1 class="text-2xl font-bold text-gray-900">Resolve Conflict</h1>
    </div>

    <div v-if="loading" class="py-8 text-center text-gray-400">Loading…</div>

    <template v-else-if="conflict">
      <div class="bg-white rounded-lg border border-gray-200 p-4 mb-6 text-sm text-gray-600 space-y-1">
        <p><span class="font-medium text-gray-900">Provider:</span> {{ conflict.provider_type }}</p>
        <p><span class="font-medium text-gray-900">Remote ID:</span> <span class="font-mono">{{ conflict.remote_id }}</span></p>
        <p><span class="font-medium text-gray-900">Created:</span> {{ formatDate(conflict.created_at) }}</p>
      </div>

      <div v-if="diffs.length === 0" class="bg-amber-50 border border-amber-200 rounded-lg p-4 text-sm text-amber-800 mb-6">
        No field-level conflicts detected. The vCards differ but no specific field diffs were recorded.
        Choosing "Apply source (remote)" will overwrite the local contact.
      </div>

      <!-- Per-field resolution -->
      <div v-else class="space-y-4 mb-6">
        <h2 class="text-sm font-semibold text-gray-700 uppercase tracking-wide">Choose per field</h2>
        <div
          v-for="diff in diffs"
          :key="diff.field"
          class="bg-white rounded-lg border border-gray-200 overflow-hidden"
        >
          <div class="px-4 py-2 bg-gray-50 border-b border-gray-200 flex items-center justify-between">
            <span class="text-xs font-semibold text-gray-700 uppercase tracking-wide">{{ diff.field }}</span>
            <span v-if="diff.base" class="text-xs text-gray-400">base: {{ diff.base }}</span>
          </div>
          <div class="flex divide-x divide-gray-200">
            <label
              class="flex-1 flex items-start gap-3 p-4 cursor-pointer hover:bg-blue-50 transition-colors"
              :class="{ 'bg-blue-50 ring-1 ring-inset ring-blue-400': resolution[diff.field] !== 'remote' }"
            >
              <input
                type="radio"
                :name="`field-${diff.field}`"
                value="local"
                v-model="resolution[diff.field]"
                class="mt-0.5 shrink-0"
              />
              <div class="min-w-0">
                <p class="text-xs font-medium text-blue-700 mb-1">Keep local</p>
                <p class="text-sm text-gray-800 break-all">{{ diff.local || '(empty)' }}</p>
              </div>
            </label>
            <label
              class="flex-1 flex items-start gap-3 p-4 cursor-pointer hover:bg-green-50 transition-colors"
              :class="{ 'bg-green-50 ring-1 ring-inset ring-green-400': resolution[diff.field] === 'remote' }"
            >
              <input
                type="radio"
                :name="`field-${diff.field}`"
                value="remote"
                v-model="resolution[diff.field]"
                class="mt-0.5 shrink-0"
              />
              <div class="min-w-0">
                <p class="text-xs font-medium text-green-700 mb-1">Use remote</p>
                <p class="text-sm text-gray-800 break-all">{{ diff.remote || '(empty)' }}</p>
              </div>
            </label>
          </div>
        </div>
      </div>

      <!-- Quick-resolve buttons when no specific diffs -->
      <div v-if="diffs.length === 0" class="flex gap-3 mb-4">
        <button
          class="flex-1 py-2 px-4 rounded-lg border-2 border-blue-300 bg-blue-50 text-blue-800 text-sm font-medium hover:bg-blue-100 transition-colors"
          @click="resolveAllLocal"
        >
          Keep local (dest wins)
        </button>
        <button
          class="flex-1 py-2 px-4 rounded-lg border-2 border-green-300 bg-green-50 text-green-800 text-sm font-medium hover:bg-green-100 transition-colors"
          @click="resolveAllRemote"
        >
          Apply remote (source wins)
        </button>
      </div>

      <div class="flex justify-end gap-3">
        <AppButton variant="secondary" @click="router.back()">Cancel</AppButton>
        <AppButton :loading="saving" @click="handleResolve">Save Resolution</AppButton>
      </div>

      <p v-if="errorMsg" class="mt-3 text-sm text-red-600">{{ errorMsg }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getConflict, resolveConflict } from '@/api/sync'
import type { SyncConflict, FieldDiff } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const loading = ref(true)
const saving = ref(false)
const errorMsg = ref('')
const conflict = ref<SyncConflict | null>(null)
const diffs = ref<FieldDiff[]>([])
const resolution = reactive<Record<string, string>>({})

function resolveAllLocal() {
  diffs.value.forEach(d => { resolution[d.field] = 'local' })
  if (diffs.value.length === 0) handleResolve()
}

function resolveAllRemote() {
  diffs.value.forEach(d => { resolution[d.field] = 'remote' })
  if (diffs.value.length === 0) {
    // No diffs stored — signal full remote override
    resolution['*'] = 'remote'
    handleResolve()
  }
}

async function handleResolve() {
  if (!conflict.value) return
  saving.value = true
  errorMsg.value = ''
  try {
    await resolveConflict(id, resolution)
    router.push({ name: 'sync-conflicts' })
  } catch (e: unknown) {
    errorMsg.value = (e as { response?: { data?: { error?: string } } })?.response?.data?.error ?? 'Failed to resolve conflict'
  } finally {
    saving.value = false
  }
}

function formatDate(iso: string) {
  return new Date(iso).toLocaleString()
}

onMounted(async () => {
  try {
    const { data } = await getConflict(id)
    conflict.value = data
    try {
      diffs.value = JSON.parse(data.field_diffs) as FieldDiff[]
    } catch {
      diffs.value = []
    }
    // Default all conflicting fields to "local"
    diffs.value.forEach(d => { resolution[d.field] = 'local' })
  } finally {
    loading.value = false
  }
})
</script>
