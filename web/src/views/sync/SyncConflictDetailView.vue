<template>
  <div class="max-w-3xl">
    <div class="flex items-center gap-3 mb-6">
      <button @click="router.back()" class="text-muted-foreground hover:text-foreground">
        <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" />
        </svg>
      </button>
      <h1 class="text-2xl font-bold text-foreground">Resolve Conflict</h1>
    </div>

    <div v-if="loading" class="py-8 text-center text-muted-foreground">Loading...</div>

    <template v-else-if="conflict">
      <div class="bg-card rounded-lg border border-border p-4 mb-6 text-sm text-muted-foreground space-y-1">
        <p><span class="font-medium text-foreground">Provider:</span> {{ conflict.provider_type }}</p>
        <p><span class="font-medium text-foreground">Remote ID:</span> <span class="font-mono">{{ conflict.remote_id }}</span></p>
        <p><span class="font-medium text-foreground">Created:</span> {{ formatDateTime(conflict.created_at) }}</p>
      </div>

      <div v-if="diffs.length === 0" class="bg-amber-50 dark:bg-amber-500/20 border border-amber-200 dark:border-amber-500/30 rounded-lg p-4 text-sm text-amber-800 dark:text-amber-300 mb-6">
        No field-level conflicts detected. The vCards differ but no specific field diffs were recorded.
        Choosing "Apply source (remote)" will overwrite the local contact.
      </div>

      <!-- Per-field resolution -->
      <div v-else class="space-y-4 mb-6">
        <h2 class="text-sm font-semibold text-foreground uppercase tracking-wide">Choose per field</h2>
        <div
          v-for="diff in diffs"
          :key="diff.field"
          class="bg-card rounded-lg border border-border overflow-hidden"
        >
          <div class="px-4 py-2 bg-muted/50 border-b border-border flex items-center justify-between">
            <span class="text-xs font-semibold text-foreground uppercase tracking-wide">{{ diff.field }}</span>
            <span v-if="diff.base" class="text-xs text-muted-foreground">base: {{ diff.base }}</span>
          </div>
          <div class="flex divide-x divide-border">
            <label
              class="flex-1 flex items-start gap-3 p-4 cursor-pointer hover:bg-blue-50 dark:hover:bg-blue-500/10 transition-colors"
              :class="{ 'bg-blue-50 dark:bg-blue-500/10 ring-1 ring-inset ring-blue-400': resolution[diff.field] !== 'remote' }"
            >
              <input
                type="radio"
                :name="`field-${diff.field}`"
                value="local"
                v-model="resolution[diff.field]"
                class="mt-0.5 shrink-0"
              />
              <div class="min-w-0">
                <p class="text-xs font-medium text-blue-700 dark:text-blue-400 mb-1">Keep local</p>
                <p class="text-sm text-foreground break-all">{{ diff.local || '(empty)' }}</p>
              </div>
            </label>
            <label
              class="flex-1 flex items-start gap-3 p-4 cursor-pointer hover:bg-green-50 dark:hover:bg-green-500/10 transition-colors"
              :class="{ 'bg-green-50 dark:bg-green-500/10 ring-1 ring-inset ring-green-400': resolution[diff.field] === 'remote' }"
            >
              <input
                type="radio"
                :name="`field-${diff.field}`"
                value="remote"
                v-model="resolution[diff.field]"
                class="mt-0.5 shrink-0"
              />
              <div class="min-w-0">
                <p class="text-xs font-medium text-green-700 dark:text-green-400 mb-1">Use remote</p>
                <p class="text-sm text-foreground break-all">{{ diff.remote || '(empty)' }}</p>
              </div>
            </label>
          </div>
        </div>
      </div>

      <!-- Quick-resolve buttons when no specific diffs -->
      <div v-if="diffs.length === 0" class="flex gap-3 mb-4">
        <button
          class="flex-1 py-2 px-4 rounded-lg border-2 border-blue-300 dark:border-blue-500/50 bg-blue-50 dark:bg-blue-500/10 text-blue-800 dark:text-blue-300 text-sm font-medium hover:bg-blue-100 dark:hover:bg-blue-500/20 transition-colors"
          @click="resolveAllLocal"
        >
          Keep local (dest wins)
        </button>
        <button
          class="flex-1 py-2 px-4 rounded-lg border-2 border-green-300 dark:border-green-500/50 bg-green-50 dark:bg-green-500/10 text-green-800 dark:text-green-300 text-sm font-medium hover:bg-green-100 dark:hover:bg-green-500/20 transition-colors"
          @click="resolveAllRemote"
        >
          Apply remote (source wins)
        </button>
      </div>

      <div class="flex justify-end gap-3">
        <AppButton variant="secondary" @click="router.back()">Cancel</AppButton>
        <AppButton :loading="saving" @click="handleResolve">Save Resolution</AppButton>
      </div>

      <p v-if="errorMsg" class="mt-3 text-sm text-destructive">{{ errorMsg }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getConflict, resolveConflict } from '@/api/sync'
import type { SyncConflict, FieldDiff } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import { formatDateTime } from '@/utils/date'

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
