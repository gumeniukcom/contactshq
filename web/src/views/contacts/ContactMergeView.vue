<template>
  <div class="max-w-2xl">
    <div class="flex items-center gap-3 mb-6">
      <RouterLink to="/contacts/duplicates" class="text-sm text-muted-foreground hover:text-foreground">
        ← Duplicates
      </RouterLink>
      <h1 class="text-2xl font-bold text-foreground">Advanced Merge</h1>
    </div>

    <div v-if="loading" class="py-8 text-center text-muted-foreground">Loading…</div>
    <div v-else-if="!dup" class="py-8 text-center text-muted-foreground">Duplicate pair not found.</div>

    <template v-else>
      <!-- Score header -->
      <div class="mb-4 flex items-center gap-3">
        <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-orange-100 dark:bg-orange-500/20 text-orange-700 dark:text-orange-300">
          {{ Math.round(dup.score * 100) }}% match
        </span>
        <span class="text-xs text-muted-foreground">{{ parsedReasons.join(', ') }}</span>
      </div>

      <!-- Field-by-field picker -->
      <div class="bg-card rounded-lg border border-border overflow-hidden mb-6">
        <div class="grid grid-cols-3 bg-muted/50 border-b border-border text-xs font-semibold text-muted-foreground uppercase px-4 py-2">
          <span>Field</span>
          <span>Contact A <button class="ml-1 text-accent hover:underline" @click="keepAll('a')">Keep all A</button></span>
          <span>Contact B <button class="ml-1 text-accent hover:underline" @click="keepAll('b')">Keep all B</button></span>
        </div>

        <div
          v-for="field in fields"
          :key="field.key"
          class="grid grid-cols-3 px-4 py-3 border-b border-border last:border-b-0 items-start"
          :class="{ 'bg-yellow-50 dark:bg-yellow-500/10': field.differs }"
        >
          <span class="text-xs font-medium text-muted-foreground pt-0.5 capitalize">{{ field.label }}</span>

          <!-- Contact A option -->
          <label class="flex items-start gap-2 cursor-pointer">
            <input
              type="radio"
              :name="field.key"
              value="a"
              v-model="resolution[field.key]"
              class="mt-0.5 text-accent"
            />
            <span class="text-sm text-foreground break-all">{{ field.valueA || '—' }}</span>
          </label>

          <!-- Contact B option -->
          <label class="flex items-start gap-2 cursor-pointer">
            <input
              type="radio"
              :name="field.key"
              value="b"
              v-model="resolution[field.key]"
              class="mt-0.5 text-accent"
            />
            <span class="text-sm text-foreground break-all">{{ field.valueB || '—' }}</span>
          </label>
        </div>
      </div>

      <!-- Actions -->
      <div class="flex gap-3">
        <AppButton :loading="merging" @click="doMerge">Merge contacts</AppButton>
        <RouterLink
          to="/contacts/duplicates"
          class="inline-flex items-center px-4 py-2 text-sm font-medium rounded-md border border-input bg-card text-foreground hover:bg-muted/50"
        >
          Cancel
        </RouterLink>
      </div>

      <p v-if="error" class="mt-3 text-sm text-destructive">{{ error }}</p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { listDuplicates, mergeContacts } from '@/api/contacts'
import type { PotentialDuplicate, Contact } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'

const route = useRoute()
const router = useRouter()

const loading = ref(false)
const merging = ref(false)
const error = ref('')
const dup = ref<PotentialDuplicate | null>(null)

const FIELD_LABELS: Record<string, string> = {
  first_name: 'First name',
  last_name: 'Last name',
  email: 'Email',
  phone: 'Phone',
  org: 'Organisation',
  title: 'Title',
  note: 'Note',
}

interface FieldRow {
  key: keyof Contact
  label: string
  valueA: string
  valueB: string
  differs: boolean
}

const fields = computed<FieldRow[]>(() => {
  if (!dup.value?.contact_a || !dup.value?.contact_b) return []
  const a = dup.value.contact_a
  const b = dup.value.contact_b
  return (Object.keys(FIELD_LABELS) as (keyof Contact)[]).map((key) => ({
    key,
    label: FIELD_LABELS[key as string],
    valueA: (a[key] as string) || '',
    valueB: (b[key] as string) || '',
    differs: (a[key] as string) !== (b[key] as string),
  }))
})

// resolution[fieldKey] = 'a' | 'b'
const resolution = ref<Record<string, string>>({})

function keepAll(side: 'a' | 'b') {
  fields.value.forEach((f) => { resolution.value[f.key as string] = side })
}

const parsedReasons = computed<string[]>(() => {
  if (!dup.value) return []
  try { return JSON.parse(dup.value.match_reasons) as string[] } catch { return [] }
})

async function fetchDup() {
  loading.value = true
  try {
    const dupId = route.params.dupId as string
    const { data } = await listDuplicates({ status: '', limit: 200, offset: 0 })
    dup.value = data.duplicates.find((d) => d.id === dupId) ?? null
    if (dup.value) {
      // Default: pick A for all fields
      fields.value.forEach((f) => { resolution.value[f.key as string] = 'a' })
    }
  } finally {
    loading.value = false
  }
}

async function doMerge() {
  if (!dup.value) return
  merging.value = true
  error.value = ''

  // Build winner/loser: majority vote of resolution map
  const aCount = Object.values(resolution.value).filter((v) => v === 'a').length
  const bCount = Object.values(resolution.value).filter((v) => v === 'b').length
  const winnerId = aCount >= bCount ? dup.value.contact_a_id : dup.value.contact_b_id
  const loserId  = aCount >= bCount ? dup.value.contact_b_id : dup.value.contact_a_id

  // Map field resolutions to winner/loser keys
  const fieldResolution: Record<string, string> = {}
  const winnerSide = aCount >= bCount ? 'a' : 'b'
  fields.value.forEach((f) => {
    fieldResolution[f.key as string] = resolution.value[f.key as string] === winnerSide ? 'winner' : 'loser'
  })

  try {
    await mergeContacts({ winner_id: winnerId, loser_id: loserId, resolution: fieldResolution })
    router.push('/contacts/duplicates')
  } catch (e: unknown) {
    error.value = (e as Error)?.message ?? 'Merge failed'
  } finally {
    merging.value = false
  }
}

onMounted(fetchDup)
</script>
