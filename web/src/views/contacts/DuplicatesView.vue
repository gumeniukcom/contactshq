<template>
  <div class="max-w-3xl space-y-6">
    <!-- Dedup Schedule Settings -->
    <AppCard>
      <h2 class="text-lg font-semibold text-foreground mb-4">Automatic Detection</h2>
      <div class="space-y-4">
        <label class="flex items-center gap-3 cursor-pointer">
          <input type="checkbox" v-model="dedupSettings.enabled" class="h-4 w-4 text-accent rounded" />
          <span class="text-sm font-medium text-foreground">Enable scheduled duplicate detection</span>
        </label>

        <ScheduleInput
          v-if="dedupSettings.enabled"
          v-model="dedupSettings.schedule"
          :presets="DEDUP_PRESETS"
          label="Schedule"
        />
      </div>

      <div class="mt-5 flex items-center gap-3">
        <AppButton :loading="savingDedupSettings" @click="handleSaveDedupSettings">Save Settings</AppButton>
        <span v-if="dedupSettingsSaved" class="text-sm text-green-600 dark:text-green-400">Settings saved</span>
      </div>
    </AppCard>

    <div class="flex items-center justify-between">
      <h1 class="text-2xl font-bold text-foreground">
        Potential Duplicates
        <span v-if="total > 0" class="ml-2 inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium bg-orange-100 dark:bg-orange-500/20 text-orange-800 dark:text-orange-300">
          {{ total }}
        </span>
      </h1>
      <div class="flex gap-2">
        <select v-model="statusFilter" class="text-sm rounded-md border-input border px-3 py-2">
          <option value="pending">Pending</option>
          <option value="dismissed">Dismissed</option>
          <option value="merged">Merged</option>
          <option value="">All</option>
        </select>
        <AppButton size="sm" variant="secondary" :loading="detecting" @click="runDetect">
          Scan now
        </AppButton>
      </div>
    </div>

    <p v-if="detectMsg" class="mb-4 text-sm text-green-700 dark:text-green-300 bg-green-50 dark:bg-green-500/20 border border-green-200 dark:border-green-500/30 rounded px-3 py-2">
      {{ detectMsg }}
    </p>

    <div v-if="loading" class="py-8 text-center text-muted-foreground">Loading…</div>
    <div v-else-if="duplicates.length === 0" class="py-12 text-center text-muted-foreground">
      <svg class="mx-auto h-12 w-12 text-muted-foreground/60 mb-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5"
          d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
      </svg>
      <p>No duplicate pairs found.</p>
    </div>

    <div v-else class="space-y-4">
      <div
        v-for="dup in duplicates"
        :key="dup.id"
        class="bg-card rounded-lg border border-border overflow-hidden"
      >
        <!-- Header row -->
        <div class="px-4 py-3 bg-muted/50 border-b border-border flex items-center justify-between">
          <div class="flex items-center gap-3">
            <span class="inline-flex items-center px-2 py-0.5 rounded text-xs font-semibold bg-orange-100 dark:bg-orange-500/20 text-orange-700 dark:text-orange-300">
              {{ Math.round(dup.score * 100) }}% match
            </span>
            <span class="text-xs text-muted-foreground">{{ parsedReasons(dup).join(', ') }}</span>
          </div>
          <div class="flex gap-3">
            <button
              v-if="dup.status === 'pending'"
              class="text-xs text-muted-foreground hover:text-foreground"
              @click="dismiss(dup.id)"
            >Dismiss</button>
          </div>
        </div>

        <!-- Contact pair comparison -->
        <div v-if="dup.contact_a && dup.contact_b" class="grid grid-cols-2 divide-x divide-border">
          <!-- Contact A -->
          <div class="p-4">
            <p class="text-xs font-semibold text-muted-foreground uppercase mb-2">Contact A</p>
            <ContactSummary :contact="dup.contact_a" />
          </div>
          <!-- Contact B -->
          <div class="p-4">
            <p class="text-xs font-semibold text-muted-foreground uppercase mb-2">Contact B</p>
            <ContactSummary :contact="dup.contact_b" />
          </div>
        </div>

        <!-- Merge controls (only for pending) -->
        <div v-if="dup.status === 'pending' && dup.contact_a && dup.contact_b" class="px-4 py-3 border-t border-border bg-muted/50 flex gap-3">
          <AppButton size="sm" :loading="mergingId === dup.id + '_a'" @click="quickMerge(dup, 'a')">
            Keep A
          </AppButton>
          <AppButton size="sm" variant="secondary" :loading="mergingId === dup.id + '_b'" @click="quickMerge(dup, 'b')">
            Keep B
          </AppButton>
          <RouterLink
            :to="{ name: 'contact-merge', params: { dupId: dup.id } }"
            class="inline-flex items-center px-3 py-1.5 text-xs font-medium rounded-md border border-input bg-card text-foreground hover:bg-muted/50"
          >
            Advanced merge
          </RouterLink>
        </div>
      </div>
    </div>

    <!-- Pagination -->
    <div v-if="total > limit" class="mt-6 flex justify-center gap-2">
      <AppButton size="sm" variant="secondary" :disabled="offset === 0" @click="prevPage">Previous</AppButton>
      <span class="text-sm text-muted-foreground self-center">{{ page + 1 }} / {{ Math.ceil(total / limit) }}</span>
      <AppButton size="sm" variant="secondary" :disabled="offset + limit >= total" @click="nextPage">Next</AppButton>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, defineComponent, h } from 'vue'
import { RouterLink } from 'vue-router'
import { listDuplicates, dismissDuplicate, detectDuplicates, mergeContacts, getDedupSettings, saveDedupSettings } from '@/api/contacts'
import type { PotentialDuplicate, Contact, DedupSettings } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import ScheduleInput from '@/components/ui/ScheduleInput.vue'
import { DEDUP_PRESETS } from '@/utils/cron'

// Inline ContactSummary component to avoid extra file
const ContactSummary = defineComponent({
  props: { contact: { type: Object as () => Contact, required: true } },
  setup(props) {
    return () => h('div', { class: 'space-y-1 text-sm' }, [
      h('p', { class: 'font-medium text-foreground' },
        [props.contact.first_name, props.contact.last_name].filter(Boolean).join(' ') || '(no name)'),
      props.contact.email ? h('p', { class: 'text-muted-foreground' }, props.contact.email) : null,
      props.contact.phone ? h('p', { class: 'text-muted-foreground' }, props.contact.phone) : null,
      props.contact.org ? h('p', { class: 'text-muted-foreground text-xs' }, props.contact.org) : null,
    ].filter(Boolean))
  }
})

const loading = ref(false)
const detecting = ref(false)
const detectMsg = ref('')
const duplicates = ref<PotentialDuplicate[]>([])
const total = ref(0)
const limit = 20
const offset = ref(0)
const page = ref(0)
const statusFilter = ref('pending')
const mergingId = ref('')

async function fetchDuplicates() {
  loading.value = true
  try {
    const { data } = await listDuplicates({ status: statusFilter.value || undefined, limit, offset: offset.value })
    duplicates.value = data.duplicates
    total.value = data.total
  } finally {
    loading.value = false
  }
}

watch(statusFilter, () => { offset.value = 0; page.value = 0; fetchDuplicates() })

function prevPage() { offset.value = Math.max(0, offset.value - limit); page.value--; fetchDuplicates() }
function nextPage() { offset.value += limit; page.value++; fetchDuplicates() }

async function dismiss(id: string) {
  await dismissDuplicate(id)
  fetchDuplicates()
}

async function runDetect() {
  detecting.value = true
  detectMsg.value = ''
  try {
    const { data } = await detectDuplicates()
    detectMsg.value = `Found ${data.found} new duplicate pair(s) out of ${data.checked} contacts checked.`
    fetchDuplicates()
  } finally {
    detecting.value = false
  }
}

// Quick merge: pick one contact as winner, other as loser, no field-level choices
async function quickMerge(dup: PotentialDuplicate, keep: 'a' | 'b') {
  const winnerId = keep === 'a' ? dup.contact_a_id : dup.contact_b_id
  const loserId  = keep === 'a' ? dup.contact_b_id : dup.contact_a_id
  mergingId.value = dup.id + '_' + keep
  try {
    await mergeContacts({ winner_id: winnerId, loser_id: loserId, resolution: {} })
    fetchDuplicates()
  } finally {
    mergingId.value = ''
  }
}

function parsedReasons(dup: PotentialDuplicate): string[] {
  try { return JSON.parse(dup.match_reasons) as string[] } catch { return [] }
}

// ── Dedup Schedule Settings ────────────────────────────────────────────────
const dedupSettings = ref<DedupSettings>({ schedule: '0 2 * * *', enabled: false })
const savingDedupSettings = ref(false)
const dedupSettingsSaved = ref(false)

async function loadDedupSettings() {
  try {
    const { data } = await getDedupSettings()
    dedupSettings.value = { schedule: data.schedule, enabled: data.enabled }
  } catch {
    // keep defaults
  }
}

async function handleSaveDedupSettings() {
  savingDedupSettings.value = true
  dedupSettingsSaved.value = false
  try {
    await saveDedupSettings(dedupSettings.value)
    dedupSettingsSaved.value = true
    setTimeout(() => { dedupSettingsSaved.value = false }, 3000)
  } finally {
    savingDedupSettings.value = false
  }
}

onMounted(() => {
  fetchDuplicates()
  loadDedupSettings()
})
</script>
