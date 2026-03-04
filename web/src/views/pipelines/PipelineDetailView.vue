<template>
  <div class="max-w-2xl">
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold text-gray-900">Edit Pipeline</h1>
      <AppButton variant="secondary" @click="handleTrigger" :loading="triggering">
        Trigger Now
      </AppButton>
    </div>

    <AppCard>
      <div v-if="loadingPipeline" class="py-8 text-center text-gray-500">Loading...</div>
      <form v-else class="space-y-4" @submit.prevent="handleUpdate">
        <AppInput v-model="form.name" label="Name" id="name" />

        <div class="flex items-center gap-3">
          <label class="text-sm font-medium text-gray-700">Enabled</label>
          <input type="checkbox" v-model="form.enabled" class="rounded border-gray-300 text-indigo-600 focus:ring-indigo-500" />
        </div>

        <ScheduleInput v-model="form.schedule" :presets="SYNC_PRESETS" label="Schedule" />

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
                  <option value="pull">Pull (remote -> local)</option>
                  <option value="push">Push (local -> remote)</option>
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

            <CardDAVStepConfig
              v-if="step.dest_type === 'carddav'"
              :index="i"
              side="dst"
              v-model="step._dst"
              :credentials="cardDAVCredentials"
              class="mt-3"
            />

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
          <AppButton variant="secondary" @click="router.push({ name: 'pipeline-view', params: { id } })">Cancel</AppButton>
          <AppButton type="submit" :loading="saving">Update</AppButton>
        </div>
      </form>
    </AppCard>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getPipeline, updatePipeline, triggerPipeline } from '@/api/pipelines'
import { listCredentials } from '@/api/credentials'
import type { PipelineStep, Credential } from '@/types'
import AppCard from '@/components/ui/AppCard.vue'
import AppButton from '@/components/ui/AppButton.vue'
import ScheduleInput from '@/components/ui/ScheduleInput.vue'
import { SYNC_PRESETS } from '@/utils/cron'
import CardDAVStepConfig from './CardDAVStepConfig.vue'
import GoogleStepConfig from './GoogleStepConfig.vue'
import type { CardDAVConfig } from './CardDAVStepConfig.vue'
import type { GoogleConfig } from './GoogleStepConfig.vue'

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

const route = useRoute()
const router = useRouter()
const id = route.params.id as string

const loadingPipeline = ref(true)
const saving = ref(false)
const triggering = ref(false)
const cardDAVCredentials = ref<Credential[]>([])
const googleCredentials = ref<Credential[]>([])

const form = reactive({
  name: '',
  enabled: true,
  schedule: '',
  steps: [] as StepFormItem[],
})

function emptyCardDAV(): CardDAVConfig {
  return { credential_id: '', endpoint: '', username: '', password: '', skip_tls_verify: false }
}

function emptyGoogle(): GoogleConfig {
  return { credential_id: '' }
}

function parseCardDAV(configJSON?: string): CardDAVConfig {
  if (!configJSON || configJSON === '{}') return emptyCardDAV()
  try {
    const parsed = JSON.parse(configJSON)
    return {
      credential_id: parsed.credential_id ?? '',
      endpoint: parsed.endpoint ?? '',
      username: parsed.username ?? '',
      password: parsed.password ?? '',
      skip_tls_verify: parsed.skip_tls_verify ?? false,
    }
  } catch {
    return emptyCardDAV()
  }
}

function parseGoogle(configJSON?: string): GoogleConfig {
  if (!configJSON || configJSON === '{}') return emptyGoogle()
  try {
    const parsed = JSON.parse(configJSON)
    return { credential_id: parsed.credential_id ?? '' }
  } catch {
    return emptyGoogle()
  }
}

function stepToFormItem(step: PipelineStep): StepFormItem {
  return {
    source_type: step.source_type,
    dest_type: step.dest_type,
    conflict_mode: step.conflict_mode,
    direction: step.direction ?? 'pull',
    _src: parseCardDAV(step.source_config),
    _dst: parseCardDAV(step.dest_config),
    _gsrc: parseGoogle(step.source_config),
    _gdst: parseGoogle(step.dest_config),
  }
}

function addStep() {
  form.steps.push({
    source_type: 'internal', dest_type: 'carddav', conflict_mode: 'auto', direction: 'pull',
    _src: emptyCardDAV(), _dst: emptyCardDAV(), _gsrc: emptyGoogle(), _gdst: emptyGoogle(),
  })
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

function buildSteps(): PipelineStep[] {
  return form.steps.map(s => {
    let source_config = '{}'
    if (s.source_type === 'carddav') source_config = buildStepConfig(s._src)
    else if (s.source_type === 'google') source_config = buildGoogleConfig(s._gsrc)

    let dest_config = '{}'
    if (s.dest_type === 'carddav') dest_config = buildStepConfig(s._dst)
    else if (s.dest_type === 'google') dest_config = buildGoogleConfig(s._gdst)

    return {
      source_type: s.source_type,
      dest_type: s.dest_type,
      conflict_mode: s.conflict_mode as PipelineStep['conflict_mode'],
      direction: s.direction as PipelineStep['direction'],
      source_config,
      dest_config,
    }
  })
}

onMounted(async () => {
  try {
    const [pipelineRes, cardDAVRes, googleRes] = await Promise.allSettled([
      getPipeline(id),
      listCredentials({ type: 'carddav' }),
      listCredentials({ type: 'google' }),
    ])
    if (pipelineRes.status === 'fulfilled') {
      const data = pipelineRes.value.data
      form.name = data.name
      form.enabled = data.enabled
      form.schedule = data.schedule
      form.steps = (data.steps ?? []).map(stepToFormItem)
    }
    if (cardDAVRes.status === 'fulfilled') {
      cardDAVCredentials.value = cardDAVRes.value.data.credentials ?? []
    }
    if (googleRes.status === 'fulfilled') {
      googleCredentials.value = googleRes.value.data.credentials ?? []
    }
  } finally {
    loadingPipeline.value = false
  }
})

async function handleUpdate() {
  saving.value = true
  try {
    await updatePipeline(id, { ...form, steps: buildSteps() })
    router.push({ name: 'pipeline-view', params: { id } })
  } finally {
    saving.value = false
  }
}

async function handleTrigger() {
  triggering.value = true
  try {
    await triggerPipeline(id)
  } finally {
    triggering.value = false
  }
}
</script>
