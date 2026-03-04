<template>
  <div class="p-3 bg-muted/50 rounded border border-border space-y-2">
    <p class="text-xs font-semibold text-muted-foreground uppercase tracking-wide">
      {{ side === 'src' ? 'Source' : 'Destination' }} CardDAV
    </p>

    <!-- Credential selector -->
    <div>
      <label class="block text-xs text-muted-foreground mb-1">Credential</label>
      <select
        :value="modelValue.credential_id"
        class="block w-full rounded-md border-input text-sm px-3 py-2 border"
        @change="onCredentialChange"
      >
        <option value="">— Enter manually —</option>
        <option v-for="cred in credentials" :key="cred.id" :value="cred.id">
          {{ cred.name }} — {{ cred.endpoint }}
        </option>
      </select>
    </div>

    <!-- Manual fields (only when no credential selected) -->
    <template v-if="!modelValue.credential_id">
      <AppInput
        :model-value="modelValue.endpoint"
        label="Server URL"
        placeholder="https://dav.example.com/addressbooks/user/"
        :id="`${side}-url-${index}`"
        @update:model-value="update('endpoint', $event)"
      />
      <AppInput
        :model-value="modelValue.username"
        label="Username"
        :id="`${side}-user-${index}`"
        @update:model-value="update('username', $event)"
      />
      <AppInput
        :model-value="modelValue.password"
        label="Password"
        type="password"
        :id="`${side}-pass-${index}`"
        @update:model-value="update('password', $event)"
      />
      <div class="flex items-center gap-2">
        <input
          :id="`${side}-tls-${index}`"
          :checked="modelValue.skip_tls_verify"
          type="checkbox"
          class="rounded border-input text-accent focus:ring-ring"
          @change="update('skip_tls_verify', ($event.target as HTMLInputElement).checked)"
        />
        <label :for="`${side}-tls-${index}`" class="text-xs text-muted-foreground">
          Skip TLS/SSL verification
        </label>
      </div>
    </template>

    <!-- Credential summary when selected -->
    <div v-else class="text-xs text-muted-foreground flex items-center gap-1">
      <svg class="h-3.5 w-3.5 text-green-500 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
      </svg>
      Using saved credential "{{ selectedCredentialName }}"
      <RouterLink to="/credentials" class="text-accent hover:text-accent/80 hover:underline ml-1">manage</RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { RouterLink } from 'vue-router'
import type { Credential } from '@/types'
import AppInput from '@/components/ui/AppInput.vue'

export interface CardDAVConfig {
  credential_id: string
  endpoint: string
  username: string
  password: string
  skip_tls_verify: boolean
}

const props = defineProps<{
  modelValue: CardDAVConfig
  credentials: Credential[]
  side: 'src' | 'dst'
  index: number
}>()

const emit = defineEmits<{
  'update:modelValue': [value: CardDAVConfig]
}>()

const selectedCredentialName = computed(() =>
  props.credentials.find(c => c.id === props.modelValue.credential_id)?.name ?? ''
)

function update<K extends keyof CardDAVConfig>(key: K, value: CardDAVConfig[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function onCredentialChange(e: Event) {
  const credId = (e.target as HTMLSelectElement).value
  emit('update:modelValue', {
    credential_id: credId,
    endpoint: '',
    username: '',
    password: '',
    skip_tls_verify: false,
  })
}
</script>
