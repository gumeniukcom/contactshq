<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-gray-900 mb-6">Create Pipeline</h1>

    <AppCard>
      <form class="space-y-4" @submit.prevent="handleCreate">
        <AppInput v-model="form.name" label="Name" placeholder="My sync pipeline" id="name" />

        <div class="flex items-center gap-3">
          <label class="text-sm font-medium text-gray-700">Enabled</label>
          <input type="checkbox" v-model="form.enabled" class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500" />
        </div>

        <AppInput v-model="form.schedule" label="Schedule (cron)" placeholder="0 */6 * * *" id="schedule" />

        <div class="border-t border-gray-200 pt-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-medium text-gray-900">Steps</h3>
            <AppButton size="sm" variant="secondary" @click="addStep">Add Step</AppButton>
          </div>
          <div v-for="(step, i) in form.steps" :key="i" class="mb-4 p-3 rounded-md border border-gray-200">
            <div class="flex gap-3 items-end">
              <div class="flex-1">
                <label class="block text-xs text-gray-500 mb-1">Source</label>
                <select v-model="step.source_type" class="block w-full rounded-md border-gray-300 text-sm px-3 py-2 border">
                  <option value="internal">Internal</option>
                  <option value="carddav">CardDAV</option>
                  <option value="google">Google</option>
                </select>
              </div>
              <div class="flex-1">
                <label class="block text-xs text-gray-500 mb-1">Destination</label>
                <select v-model="step.dest_type" class="block w-full rounded-md border-gray-300 text-sm px-3 py-2 border">
                  <option value="internal">Internal</option>
                  <option value="carddav">CardDAV</option>
                  <option value="google">Google</option>
                </select>
              </div>
              <div class="flex-1">
                <label class="block text-xs text-gray-500 mb-1">Direction</label>
                <select v-model="step.direction" class="block w-full rounded-md border-gray-300 text-sm px-3 py-2 border">
                  <option value="pull">Pull (remote → local)</option>
                  <option value="push">Push (local → remote)</option>
                  <option value="bidirectional">Bidirectional</option>
                </select>
              </div>
              <div class="flex-1">
                <label class="block text-xs text-gray-500 mb-1">Conflict</label>
                <select v-model="step.conflict_mode" class="block w-full rounded-md border-gray-300 text-sm px-3 py-2 border">
                  <option value="auto">Auto-merge (three-way)</option>
                  <option value="source_wins">Source wins</option>
                  <option value="dest_wins">Destination wins</option>
                  <option value="skip">Skip on conflict</option>
                  <option value="manual">Always ask user</option>
                </select>
              </div>
              <button type="button" class="text-red-500 hover:text-red-700 pb-2" @click="form.steps.splice(i, 1)">
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>

            <!-- Source CardDAV config -->
            <CardDAVStepConfig
              v-if="step.source_type === 'carddav'"
              :index="i"
              side="src"
              v-model="step._src"
              :credentials="cardDAVCredentials"
              class="mt-3"
            />

            <!-- Source Google config -->
            <GoogleStepConfig
              v-if="step.source_type === 'google'"
              :index="i"
              side="src"
              v-model="step._gsrc"
              :credentials="googleCredentials"
              class="mt-3"
            />

            <!-- Dest CardDAV config -->
            <CardDAVStepConfig
              v-if="step.dest_type === 'carddav'"
              :index="i"
              side="dst"
              v-model="step._dst"
              :credentials="cardDAVCredentials"
              class="mt-3"
            />

            <!-- Dest Google config -->
            <GoogleStepConfig
              v-if="step.dest_type === 'google'"
              :index="i"
              side="dst"
              v-model="step._gdst"
              :credentials="googleCredentials"
              class="mt-3"
            />
          </div>
        </div>

        <div class="flex justify-end gap-3 pt-4">
          <AppButton variant="secondary" @click="router.back()">Cancel</AppButton>
          <AppButton type="submit" :loading="loading">Create</AppButton>
        </div>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { createPipeline } from '@/api/pipelines'
import { listCredentials } from '@/api/credentials'
import type { PipelineStep, Credential } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppButton from '@/components/ui/AppButton.vue'
import CardDAVStepConfig from './CardDAVStepConfig.vue'
import GoogleStepConfig from './GoogleStepConfig.vue'

export interface CardDAVConfig {
  credential_id: string
  endpoint: string
  username: string
  password: string
  skip_tls_verify: boolean
}

export interface GoogleConfig {
  credential_id: string
}

interface StepFormItem {
  source_type: string
  dest_type: string
  conflict_mode: string
  direction: string
  _src: CardDAVConfig
  _dst: CardDAVConfig
  _gsrc: GoogleConfig
  _gdst: GoogleConfig
}

const router = useRouter()
const loading = ref(false)
const cardDAVCredentials = ref<Credential[]>([])
const googleCredentials = ref<Credential[]>([])

function emptyCardDAV(): CardDAVConfig {
  return { credential_id: '', endpoint: '', username: '', password: '', skip_tls_verify: false }
}

function emptyGoogle(): GoogleConfig {
  return { credential_id: '' }
}

function defaultStep(): StepFormItem {
  return { source_type: 'internal', dest_type: 'carddav', conflict_mode: 'auto', direction: 'pull', _src: emptyCardDAV(), _dst: emptyCardDAV(), _gsrc: emptyGoogle(), _gdst: emptyGoogle() }
}

const form = reactive({
  name: '',
  enabled: true,
  schedule: '',
  steps: [defaultStep()] as StepFormItem[],
})

onMounted(async () => {
  await Promise.allSettled([
    listCredentials({ type: 'carddav' }).then(({ data }) => {
      cardDAVCredentials.value = data.credentials ?? []
    }),
    listCredentials({ type: 'google' }).then(({ data }) => {
      googleCredentials.value = data.credentials ?? []
    }),
  ])
})

function addStep() {
  form.steps.push(defaultStep())
}

function buildStepConfig(cfg: CardDAVConfig): string {
  if (cfg.credential_id) {
    return JSON.stringify({ credential_id: cfg.credential_id })
  }
  return JSON.stringify({
    endpoint: cfg.endpoint,
    username: cfg.username,
    password: cfg.password,
    skip_tls_verify: cfg.skip_tls_verify,
  })
}

function buildGoogleConfig(cfg: GoogleConfig): string {
  return JSON.stringify({ credential_id: cfg.credential_id })
}

function buildSourceConfig(s: StepFormItem): string {
  if (s.source_type === 'carddav') return buildStepConfig(s._src)
  if (s.source_type === 'google') return buildGoogleConfig(s._gsrc)
  return '{}'
}

function buildDestConfig(s: StepFormItem): string {
  if (s.dest_type === 'carddav') return buildStepConfig(s._dst)
  if (s.dest_type === 'google') return buildGoogleConfig(s._gdst)
  return '{}'
}

function buildSteps(): PipelineStep[] {
  return form.steps.map(s => ({
    source_type: s.source_type,
    dest_type: s.dest_type,
    conflict_mode: s.conflict_mode as PipelineStep['conflict_mode'],
    direction: s.direction as PipelineStep['direction'],
    source_config: buildSourceConfig(s),
    dest_config: buildDestConfig(s),
  }))
}

async function handleCreate() {
  loading.value = true
  try {
    await createPipeline({ ...form, steps: buildSteps() })
    router.push({ name: 'pipelines' })
  } finally {
    loading.value = false
  }
}
</script>
