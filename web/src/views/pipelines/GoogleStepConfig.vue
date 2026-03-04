<template>
  <div class="p-3 bg-muted/50 rounded border border-border space-y-2">
    <p class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
      {{ side === 'src' ? 'Source' : 'Destination' }} Google
    </p>

    <div v-if="credentials.length > 0">
      <label class="block text-xs text-muted-foreground mb-1">Google Account</label>
      <select
        :value="modelValue.credential_id"
        class="block w-full rounded-md border-input text-sm px-3 py-2 border"
        @change="onCredentialChange"
      >
        <option value="">— Select account —</option>
        <option v-for="cred in credentials" :key="cred.id" :value="cred.id">
          {{ cred.name }}
        </option>
      </select>

      <div v-if="modelValue.credential_id" class="mt-1 text-xs text-muted-foreground flex items-center gap-1">
        <svg class="h-3.5 w-3.5 text-green-500 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
        </svg>
        Using "{{ selectedCredentialName }}"
      </div>
    </div>

    <div v-else class="text-xs text-muted-foreground">
      No Google account connected.
      <RouterLink to="/settings/google" class="text-accent hover:text-accent/80 hover:underline">
        Connect Google first
      </RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Credential } from '@/types'

export interface GoogleConfig {
  credential_id: string
}

const props = defineProps<{
  modelValue: GoogleConfig
  credentials: Credential[]
  side: 'src' | 'dst'
  index: number
}>()

const emit = defineEmits<{
  'update:modelValue': [value: GoogleConfig]
}>()

const selectedCredentialName = computed(() =>
  props.credentials.find(c => c.id === props.modelValue.credential_id)?.name ?? ''
)

function onCredentialChange(e: Event) {
  const credId = (e.target as HTMLSelectElement).value
  emit('update:modelValue', { credential_id: credId })
}
</script>
