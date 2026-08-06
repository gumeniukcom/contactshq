<template>
  <div class="max-w-3xl space-y-6">
    <h1 class="text-2xl font-bold text-foreground">Backup</h1>

    <!--
      One honest line about whether backups are working. Without it a scheduled backup can
      fail every night and nothing says so until the day it is needed.
    -->
    <div
      class="rounded-lg border px-4 py-3"
      :class="
        health.alarming
          ? 'border-red-300 dark:border-red-500/40 bg-red-50 dark:bg-red-500/10'
          : 'border-border bg-muted/40'
      "
    >
      <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
        <span
          class="text-sm font-semibold"
          :class="health.alarming ? 'text-red-700 dark:text-red-400' : 'text-foreground'"
        >
          {{ healthTitle }}
        </span>
        <span class="text-sm text-muted-foreground">{{ health.summary }}</span>
      </div>
      <p v-if="lastSuccess" class="mt-1 text-xs text-muted-foreground">
        Last successful backup {{ formatAgo(lastSuccess.started_at) }} ·
        {{ formatSize(lastSuccess.size_bytes) }} · {{ lastSuccess.contact_count }} contacts
      </p>
      <p v-if="health.error" class="mt-1 text-xs text-red-700 dark:text-red-400 break-words">
        {{ health.error }}
      </p>
    </div>

    <!-- Schedule Settings Card -->
    <AppCard>
      <h2 class="text-lg font-semibold text-foreground mb-4">Automatic Backup</h2>

      <div class="space-y-4">
        <!-- Enabled toggle -->
        <label class="flex items-center gap-3 cursor-pointer">
          <input type="checkbox" v-model="settings.enabled" class="h-4 w-4 text-accent rounded" />
          <span class="text-sm font-medium text-foreground">Enable scheduled backups</span>
        </label>

        <div v-if="settings.enabled" class="space-y-3">
          <ScheduleInput v-model="settings.schedule" :presets="BACKUP_PRESETS" label="Schedule" />

          <!-- Retention -->
          <div class="flex items-center gap-3">
            <label class="text-sm font-medium text-foreground w-40">Keep last</label>
            <input
              v-model.number="settings.retention"
              type="number"
              min="1"
              max="365"
              class="w-20 rounded-md border border-input bg-background text-foreground px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-ring"
            />
            <span class="text-sm text-muted-foreground">backups</span>
          </div>
          <p v-if="settings.retention === 1" class="text-xs text-muted-foreground -mt-1">
            Keeping 1 backup means the previous one is deleted as soon as a new one succeeds.
          </p>

          <!-- Compress -->
          <label class="flex items-center gap-3 cursor-pointer">
            <input type="checkbox" v-model="settings.compress" class="h-4 w-4 text-accent rounded" />
            <span class="text-sm font-medium text-foreground">
              Compress backups with gzip
              <span class="text-muted-foreground font-normal">(saves disk space)</span>
            </span>
          </label>
        </div>
      </div>

      <div class="mt-5 flex items-center gap-3">
        <AppButton :loading="savingSettings" @click="handleSaveSettings">Save Settings</AppButton>
        <span v-if="settingsSaved" class="text-sm text-green-600 dark:text-green-400">Settings saved</span>
      </div>
    </AppCard>

    <!-- Backup List Card -->
    <AppCard>
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold text-foreground">Backup Files</h2>
        <AppButton @click="handleCreate" :loading="creating">Create Backup Now</AppButton>
      </div>

      <AppTable :columns="columns" :rows="backups" :loading="loading" empty-text="No backups yet">
        <template #body="{ rows }">
          <tr v-for="b in rows as BackupInfo[]" :key="b.id" class="hover:bg-muted/50">
            <td class="px-4 py-4 text-sm font-medium text-foreground">{{ b.filename }}</td>
            <td class="px-4 py-4 text-sm text-muted-foreground">{{ formatSize(b.size) }}</td>
            <td class="px-4 py-4 text-sm text-muted-foreground">{{ formatDateTime(b.created_at) }}</td>
            <td class="px-4 py-4 text-sm text-right">
              <div class="flex justify-end gap-2">
                <AppButton size="sm" variant="secondary" @click="handleDownload(b)">Download</AppButton>
                <AppButton size="sm" variant="secondary" @click="openRestore(b)">Restore</AppButton>
                <AppButton size="sm" variant="danger" @click="confirmDelete(b)">Delete</AppButton>
              </div>
            </td>
          </tr>
        </template>
      </AppTable>
    </AppCard>

    <!-- Restore Modal -->
    <AppModal :show="!!restoreTarget" label="Restore from Backup" @close="closeRestore">
      <div v-if="restoreTarget">
        <h3 class="text-lg font-semibold text-foreground mb-2">Restore from Backup</h3>
        <p class="text-sm text-muted-foreground mb-4">
          Restore <span class="font-medium">{{ restoreTarget.filename }}</span>
        </p>

        <div class="space-y-3 mb-5">
          <label
            class="flex items-start gap-3 cursor-pointer p-3 rounded-lg border transition-colors"
            :class="
              restoreMode === 'merge'
                ? 'border-accent bg-accent/10'
                : 'border-border hover:border-muted-foreground'
            "
          >
            <input type="radio" v-model="restoreMode" value="merge" class="mt-0.5 text-accent" />
            <div>
              <p class="text-sm font-medium text-foreground">Merge</p>
              <p class="text-xs text-muted-foreground">
                Add contacts from backup that don't exist yet (safe, non-destructive)
              </p>
            </div>
          </label>

          <label
            class="flex items-start gap-3 cursor-pointer p-3 rounded-lg border transition-colors"
            :class="
              restoreMode === 'replace'
                ? 'border-destructive bg-red-50 dark:bg-red-500/10'
                : 'border-border hover:border-muted-foreground'
            "
          >
            <input type="radio" v-model="restoreMode" value="replace" class="mt-0.5 text-destructive" />
            <div>
              <p class="text-sm font-medium text-foreground">Replace</p>
              <p class="text-xs text-destructive">
                Delete ALL current contacts and restore from backup. This cannot be undone.
              </p>
            </div>
          </label>
        </div>

        <p
          v-if="restoreResult"
          class="text-sm text-green-700 dark:text-green-400 bg-green-50 dark:bg-green-500/10 rounded p-2 mb-3"
        >
          Done: {{ restoreResult.imported }} imported, {{ restoreResult.skipped }} skipped,
          {{ restoreResult.errors }} errors.
        </p>
        <p v-if="restoreError" class="text-sm text-destructive mb-3">{{ restoreError }}</p>

        <div class="flex justify-end gap-3">
          <div v-if="restoreMode === 'replace'" class="w-full mb-3">
            <label for="replace-confirm" class="block text-sm text-foreground mb-1">
              This deletes every current contact. Type <strong>replace</strong> to confirm.
            </label>
            <input
              id="replace-confirm"
              v-model="replaceConfirmText"
              type="text"
              autocomplete="off"
              class="block w-full rounded-md border border-input bg-background text-foreground px-3 py-2 text-sm"
            />
          </div>
          <AppButton variant="secondary" @click="closeRestore"> Cancel </AppButton>
          <AppButton
            :loading="restoring"
            :disabled="!canRestore"
            :variant="restoreMode === 'replace' ? 'danger' : 'primary'"
            @click="handleRestore"
          >
            Restore
          </AppButton>
        </div>
      </div>
    </AppModal>

    <ConfirmDialog
      :show="!!deleteTarget"
      title="Delete Backup"
      :message="`Delete ${deleteTarget?.filename}? This cannot be undone.`"
      confirm-text="Delete"
      :loading="deleting"
      @confirm="handleDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  createBackup,
  getBackupStatus,
  listBackups,
  downloadBackup,
  deleteBackup,
  restoreBackup,
  getBackupSettings,
  saveBackupSettings,
} from '@/api/backup'
import { formatDateTime } from '@/utils/date'
import { formatAgo, formatSize } from '@/utils/format'
import { backupHealth } from '@/utils/backup-health'
import type { BackupInfo, BackupSettings, BackupStatus, RestoreResult } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppTable from '@/components/ui/AppTable.vue'
import ScheduleInput from '@/components/ui/ScheduleInput.vue'
import { BACKUP_PRESETS } from '@/utils/cron'
import { useToast } from '@/composables/useToast'
import { getApiError } from '@/api/client'
import AppModal from '@/components/ui/AppModal.vue'
import ConfirmDialog from '@/components/ui/ConfirmDialog.vue'

const toast = useToast()

// ── Backups list ───────────────────────────────────────────────────────────
const backups = ref<BackupInfo[]>([])
const loading = ref(true)
const creating = ref(false)

const columns = [
  { key: 'filename', label: 'Filename' },
  { key: 'size', label: 'Size' },
  { key: 'created_at', label: 'Created' },
  { key: 'actions', label: '' },
]

async function load() {
  loading.value = true
  try {
    const { data } = await listBackups()
    backups.value = data.backups || []
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  creating.value = true
  try {
    const { data } = await createBackup()
    await Promise.all([load(), loadStatus()])
    toast.success(`Backup created — ${formatSize(data.size)}`)
  } catch (e) {
    // Without this the request failed in total silence: no toast, no console, and the
    // spinner simply stopped.
    toast.error(getApiError(e, 'Backup failed'))
  } finally {
    creating.value = false
  }
}

async function handleDownload(b: BackupInfo) {
  const { data } = await downloadBackup(b.id)
  const blob = data instanceof Blob ? data : new Blob([data])
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = b.filename
  a.click()
  URL.revokeObjectURL(url)
}

// ── Delete ─────────────────────────────────────────────────────────────────
const deleteTarget = ref<BackupInfo | null>(null)
const deleting = ref(false)

function confirmDelete(b: BackupInfo) {
  deleteTarget.value = b
}

async function handleDelete() {
  if (!deleteTarget.value) return
  deleting.value = true
  try {
    await deleteBackup(deleteTarget.value.id)
    deleteTarget.value = null
    await load()
  } finally {
    deleting.value = false
  }
}

// ── Restore ────────────────────────────────────────────────────────────────
const restoreTarget = ref<BackupInfo | null>(null)
const restoreMode = ref<'merge' | 'replace'>('merge')

// Replacing wipes the address book, the same blast radius as Delete All Contacts, which
// already asks the user to type the word.
const replaceConfirmText = ref('')
const canRestore = computed(
  () => restoreMode.value !== 'replace' || replaceConfirmText.value.trim().toLowerCase() === 'replace',
)
const restoring = ref(false)
const restoreResult = ref<RestoreResult | null>(null)
const restoreError = ref('')

function openRestore(b: BackupInfo) {
  restoreTarget.value = b
  restoreMode.value = 'merge'
  replaceConfirmText.value = ''
  restoreResult.value = null
  restoreError.value = ''
}

function closeRestore() {
  restoreTarget.value = null
  restoreResult.value = null
  restoreError.value = ''
}

async function handleRestore() {
  if (!restoreTarget.value) return
  restoring.value = true
  restoreResult.value = null
  restoreError.value = ''
  try {
    const { data } = await restoreBackup(restoreTarget.value.id, restoreMode.value)
    restoreResult.value = data
  } catch (e: unknown) {
    restoreError.value = getApiError(e, 'Restore failed')
    toast.error(restoreError.value)
  } finally {
    restoring.value = false
  }
}

// ── Settings ───────────────────────────────────────────────────────────────
const settings = ref<BackupSettings>({
  schedule: '0 2 * * *',
  retention: 7,
  enabled: true,
  compress: false,
})
const savingSettings = ref(false)
const settingsSaved = ref(false)

async function loadSettings() {
  try {
    const { data } = await getBackupSettings()
    settings.value = data
  } catch {
    // keep defaults
  }
}

async function handleSaveSettings() {
  savingSettings.value = true
  settingsSaved.value = false
  try {
    await saveBackupSettings(settings.value)
    settingsSaved.value = true
    setTimeout(() => {
      settingsSaved.value = false
    }, 3000)
  } finally {
    savingSettings.value = false
  }
}

// ── Backup health ──────────────────────────────────────────────────────────
const status = ref<BackupStatus | null>(null)

const lastSuccess = computed(() => status.value?.last_success ?? null)

const health = computed(() =>
  backupHealth(settings.value, status.value?.last_success, status.value?.last_run),
)

const HEALTH_TITLES: Record<string, string> = {
  healthy: 'Backups are healthy',
  failing: 'Backups are failing',
  overdue: 'Backup overdue',
  never: 'No backup yet',
  disabled: 'Backups are off',
}

const healthTitle = computed(() => HEALTH_TITLES[health.value.status] ?? health.value.status)

async function loadStatus() {
  try {
    const { data } = await getBackupStatus()
    status.value = data
  } catch {
    // A missing status is not worth an error banner on a page that still works; the health
    // line simply falls back to what the settings alone can say.
    status.value = null
  }
}

// ── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([load(), loadSettings(), loadStatus()])
})

// ── Helpers ────────────────────────────────────────────────────────────────
</script>
