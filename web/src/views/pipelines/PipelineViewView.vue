<template>
  <div class="max-w-3xl space-y-6">
    <!-- Header -->
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-bold text-foreground">{{ pipeline?.name ?? '…' }}</h1>
        <p v-if="pipeline" class="mt-1 text-sm text-muted-foreground">
          {{ pipeline.schedule ? humanizeCron(pipeline.schedule) : 'No schedule' }} ·
          <AppBadge :color="pipeline.enabled ? 'green' : 'gray'">
            {{ pipeline.enabled ? 'Enabled' : 'Disabled' }}
          </AppBadge>
        </p>
      </div>
      <div class="flex gap-3">
        <AppButton variant="secondary" :loading="triggering" @click="handleTrigger">
          Trigger Now
        </AppButton>
        <AppButton @click="router.push({ name: 'pipeline-edit', params: { id } })">
          Edit
        </AppButton>
      </div>
    </div>

    <!-- Steps summary -->
    <AppCard v-if="pipeline && pipeline.steps?.length">
      <h2 class="text-sm font-semibold text-foreground mb-3">Steps</h2>
      <div class="space-y-2">
        <div
          v-for="(step, i) in pipeline.steps"
          :key="i"
          class="flex items-center gap-2 text-sm text-muted-foreground"
        >
          <span class="font-medium text-muted-foreground">{{ i + 1 }}.</span>
          <span class="font-medium">{{ step.source_type }}</span>
          <svg class="h-4 w-4 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 8l4 4m0 0l-4 4m4-4H3" />
          </svg>
          <span class="font-medium">{{ step.dest_type }}</span>
          <span class="text-muted-foreground">·</span>
          <span class="text-xs bg-muted rounded px-1.5 py-0.5">{{ step.direction }}</span>
          <span class="text-xs bg-muted rounded px-1.5 py-0.5">{{ step.conflict_mode }}</span>
        </div>
      </div>
    </AppCard>

    <!-- Trigger result -->
    <div
      v-if="triggerResult"
      class="rounded-lg border border-green-200 dark:border-green-500/30 bg-green-50 dark:bg-green-500/20 p-4 text-sm text-green-800 dark:text-green-300"
    >
      <p class="font-medium mb-1">Pipeline executed successfully</p>
      <ul class="space-y-0.5 text-xs">
        <li v-for="(r, i) in triggerResult" :key="i">
          Step {{ r.step_order }}:
          <template v-if="r.error">
            <span class="text-destructive">{{ r.error }}</span>
          </template>
          <template v-else-if="r.result">
            created {{ r.result.created }}, updated {{ r.result.updated }},
            deleted {{ r.result.deleted }}, errors {{ r.result.errors }}
          </template>
        </li>
      </ul>
    </div>
    <div
      v-if="triggerError"
      class="rounded-lg border border-red-200 dark:border-red-500/30 bg-red-50 dark:bg-red-500/20 p-4 text-sm text-red-700 dark:text-red-300"
    >
      {{ triggerError }}
    </div>

    <!-- Run history -->
    <AppCard>
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-sm font-semibold text-foreground">Run History</h2>
        <AppButton size="sm" variant="secondary" @click="loadRuns">Refresh</AppButton>
      </div>

      <div v-if="loadingRuns" class="py-6 text-center text-sm text-muted-foreground">Loading…</div>
      <div v-else-if="runs.length === 0" class="py-6 text-center text-sm text-muted-foreground">
        No runs yet. Trigger the pipeline to see results here.
      </div>
      <table v-else class="w-full text-sm">
        <thead>
          <tr class="border-b border-border text-left text-xs font-semibold text-muted-foreground uppercase tracking-wider">
            <th class="pb-2 pr-4">Status</th>
            <th class="pb-2 pr-4">Started</th>
            <th class="pb-2 pr-4">Duration</th>
            <th class="pb-2 pr-4">Created</th>
            <th class="pb-2 pr-4">Updated</th>
            <th class="pb-2 pr-4">Deleted</th>
            <th class="pb-2">Errors</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border">
          <tr v-for="run in runs" :key="run.id" class="hover:bg-muted/50">
            <td class="py-3 pr-4">
              <AppBadge :color="statusColor(run.status)">{{ run.status }}</AppBadge>
            </td>
            <td class="py-3 pr-4 text-muted-foreground whitespace-nowrap">{{ formatDateTime(run.started_at) }}</td>
            <td class="py-3 pr-4 text-muted-foreground">{{ formatDuration(run) }}</td>
            <td class="py-3 pr-4 text-foreground">{{ run.created_count }}</td>
            <td class="py-3 pr-4 text-foreground">{{ run.updated_count }}</td>
            <td class="py-3 pr-4 text-foreground">{{ run.deleted_count }}</td>
            <td class="py-3">
              <span :class="run.error_count > 0 ? 'text-destructive font-medium' : 'text-foreground'">
                {{ run.error_count }}
              </span>
            </td>
          </tr>
        </tbody>
      </table>
      <p v-if="runs.some(r => r.error_message)" class="mt-3 text-xs text-destructive">
        Last error: {{ runs.find(r => r.error_message)?.error_message }}
      </p>
    </AppCard>

    <!-- Back link -->
    <div>
      <button class="text-sm text-accent hover:text-accent/80 hover:underline" @click="router.push({ name: 'pipelines' })">
        ← Back to Pipelines
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { Pipeline, SyncRun } from '@/types'
import { getPipeline, triggerPipeline, listPipelineRuns } from '@/api/pipelines'
import { humanizeCron } from '@/utils/cron'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import { formatDateTime } from '@/utils/date'

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const pipeline = ref<Pipeline | null>(null)
const runs = ref<SyncRun[]>([])
const loadingRuns = ref(true)
const triggering = ref(false)
const triggerResult = ref<{ step_order: number; result?: { created: number; updated: number; deleted: number; errors: number }; error?: string }[] | null>(null)
const triggerError = ref('')

async function loadRuns() {
  loadingRuns.value = true
  try {
    const { data } = await listPipelineRuns(id)
    runs.value = data.runs ?? []
  } finally {
    loadingRuns.value = false
  }
}

onMounted(async () => {
  const [pRes] = await Promise.allSettled([getPipeline(id), loadRuns()])
  if (pRes.status === 'fulfilled') {
    pipeline.value = pRes.value.data
  }
})

async function handleTrigger() {
  triggering.value = true
  triggerResult.value = null
  triggerError.value = ''
  try {
    const { data } = await triggerPipeline(id)
    triggerResult.value = (data as { results: typeof triggerResult.value }).results ?? []
    await loadRuns()
  } catch (e: unknown) {
    triggerError.value = (e as Error)?.message ?? 'Pipeline execution failed'
  } finally {
    triggering.value = false
  }
}

function statusColor(status: string): 'green' | 'red' | 'gray' | 'blue' {
  if (status === 'completed') return 'green'
  if (status === 'failed') return 'red'
  if (status === 'running') return 'blue'
  return 'gray'
}


function formatDuration(run: SyncRun) {
  if (!run.finished_at) return 'running…'
  const ms = new Date(run.finished_at).getTime() - new Date(run.started_at).getTime()
  if (ms < 1000) return `${ms}ms`
  return `${(ms / 1000).toFixed(1)}s`
}
</script>
