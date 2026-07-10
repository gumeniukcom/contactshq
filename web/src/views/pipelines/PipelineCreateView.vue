<template>
  <div class="max-w-2xl">
    <h1 class="text-2xl font-bold text-foreground mb-6">Create Pipeline</h1>

    <AppCard>
      <form class="space-y-4" @submit.prevent="handleCreate">
        <AppInput v-model="form.name" label="Name" placeholder="My sync pipeline" id="name" />

        <div class="flex items-center gap-3">
          <label class="text-sm font-medium text-foreground">Enabled</label>
          <input
            type="checkbox"
            v-model="form.enabled"
            class="rounded border-input text-accent focus:ring-ring"
          />
        </div>

        <ScheduleInput v-model="form.schedule" :presets="SYNC_PRESETS" label="Schedule" />

        <div class="border-t border-border pt-4">
          <div class="flex items-center justify-between mb-3">
            <h3 class="text-sm font-medium text-foreground">Steps</h3>
            <AppButton size="sm" variant="secondary" @click="addStep">Add Step</AppButton>
          </div>
          <div v-for="(step, i) in form.steps" :key="i" class="mb-4 p-3 rounded-md border border-border">
            <div class="flex flex-wrap gap-3 items-end">
              <div class="flex-1 min-w-[10rem]">
                <label class="block text-xs text-muted-foreground mb-1">Provider</label>
                <select
                  v-model="step.source_type"
                  class="block w-full rounded-md border-input text-sm px-3 py-2 border"
                >
                  <option value="carddav">CardDAV</option>
                  <option value="google">Google</option>
                </select>
              </div>
              <div class="flex-1 min-w-[14rem]">
                <label class="block text-xs text-muted-foreground mb-1">Direction</label>
                <select
                  v-model="step.direction"
                  class="block w-full rounded-md border-input text-sm px-3 py-2 border"
                >
                  <option value="import">Import ({{ providerLabel(step.source_type) }} → ContactsHQ)</option>
                  <option value="export">Export (ContactsHQ → {{ providerLabel(step.source_type) }})</option>
                  <option value="two_way">Two-way</option>
                </select>
              </div>
              <div class="flex-1 min-w-[12rem]">
                <label class="block text-xs text-muted-foreground mb-1">Conflict</label>
                <select
                  v-model="step.conflict_mode"
                  class="block w-full rounded-md border-input text-sm px-3 py-2 border"
                >
                  <option value="auto">Auto-merge (three-way)</option>
                  <option value="source_wins">{{ providerLabel(step.source_type) }} wins</option>
                  <option value="dest_wins">ContactsHQ wins</option>
                  <option value="skip">Skip on conflict</option>
                  <option value="manual">Always ask me</option>
                </select>
              </div>
              <button
                type="button"
                class="text-destructive hover:text-destructive/80 pb-2"
                @click="form.steps.splice(i, 1)"
              >
                <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    stroke-width="2"
                    d="M6 18L18 6M6 6l12 12"
                  />
                </svg>
              </button>
            </div>

            <CardDAVStepConfig
              v-if="step.source_type === 'carddav'"
              :index="i"
              side="src"
              v-model="step._src"
              :credentials="cardDAVCredentials"
              class="mt-3"
            />

            <GoogleStepConfig
              v-if="step.source_type === 'google'"
              :index="i"
              side="src"
              v-model="step._gsrc"
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
import AppButton from '@/components/ui/AppButton.vue'
import ScheduleInput from '@/components/ui/ScheduleInput.vue'
import { SYNC_PRESETS } from '@/utils/cron'
import { providerLabel } from '@/utils/pipeline'
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

// A step always moves contacts between one external provider and the internal address
// book, which is why there is no destination picker: dest_type is always 'internal'.
interface StepFormItem {
  source_type: string
  conflict_mode: string
  direction: string
  _src: CardDAVConfig
  _gsrc: GoogleConfig
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
  return {
    source_type: 'carddav',
    conflict_mode: 'auto',
    direction: 'import',
    _src: emptyCardDAV(),
    _gsrc: emptyGoogle(),
  }
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
  if (s.source_type === 'google') return buildGoogleConfig(s._gsrc)
  return buildStepConfig(s._src)
}

function buildSteps(): PipelineStep[] {
  return form.steps.map((s) => ({
    source_type: s.source_type,
    dest_type: 'internal',
    conflict_mode: s.conflict_mode as PipelineStep['conflict_mode'],
    direction: s.direction as PipelineStep['direction'],
    source_config: buildSourceConfig(s),
    dest_config: '{}',
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
