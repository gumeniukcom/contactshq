<template>
  <div class="max-w-3xl space-y-6">
    <h1 class="text-2xl font-bold text-gray-900">Backup</h1>

    <!-- Schedule Settings Card -->
    <AppCard>
      <h2 class="text-lg font-semibold text-gray-900 mb-4">Automatic Backup</h2>

      <div class="space-y-4">
        <!-- Enabled toggle -->
        <label class="flex items-center gap-3 cursor-pointer">
          <input type="checkbox" v-model="settings.enabled" class="h-4 w-4 text-indigo-600 rounded" />
          <span class="text-sm font-medium text-gray-700">Enable scheduled backups</span>
        </label>

        <!-- Schedule presets -->
        <div v-if="settings.enabled" class="space-y-3">
          <div>
            <label class="block text-sm font-medium text-gray-700 mb-1">Schedule</label>
            <div class="flex flex-wrap gap-2">
              <button
                v-for="preset in schedulePresets"
                :key="preset.value"
                @click="applyPreset(preset)"
                class="px-3 py-1.5 text-xs font-medium rounded-full border transition-colors"
                :class="
                  activePreset === preset.value
                    ? 'bg-indigo-600 text-white border-indigo-600'
                    : 'bg-white text-gray-600 border-gray-300 hover:border-indigo-400'
                "
              >
                {{ preset.label }}
              </button>
            </div>
          </div>

          <!-- Custom cron input -->
          <div v-if="activePreset === 'custom'" class="mt-2">
            <label class="block text-xs font-medium text-gray-500 mb-1">
              Cron expression
              <span class="text-gray-400 font-normal ml-1">e.g. 0 2 * * * (daily at 2 AM)</span>
            </label>
            <input
              v-model="settings.schedule"
              type="text"
              placeholder="0 2 * * *"
              class="block w-full rounded-md border border-gray-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500 focus:border-indigo-500 font-mono"
            />
          </div>

          <!-- Retention -->
          <div class="flex items-center gap-3">
            <label class="text-sm font-medium text-gray-700 w-40">Keep last</label>
            <input
              v-model.number="settings.retention"
              type="number"
              min="1"
              max="365"
              class="w-20 rounded-md border border-gray-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-indigo-500"
            />
            <span class="text-sm text-gray-500">backups</span>
          </div>

          <!-- Compress -->
          <label class="flex items-center gap-3 cursor-pointer">
            <input type="checkbox" v-model="settings.compress" class="h-4 w-4 text-indigo-600 rounded" />
            <span class="text-sm font-medium text-gray-700">
              Compress backups with gzip
              <span class="text-gray-400 font-normal">(saves disk space)</span>
            </span>
          </label>
        </div>
      </div>

      <div class="mt-5 flex items-center gap-3">
        <AppButton :loading="savingSettings" @click="handleSaveSettings">Save Settings</AppButton>
        <span v-if="settingsSaved" class="text-sm text-green-600">Settings saved</span>
      </div>
    </AppCard>

    <!-- Backup List Card -->
    <AppCard>
      <div class="flex items-center justify-between mb-4">
        <h2 class="text-lg font-semibold text-gray-900">Backup Files</h2>
        <AppButton @click="handleCreate" :loading="creating">Create Backup Now</AppButton>
      </div>

      <AppTable :columns="columns" :rows="backups" :loading="loading" empty-text="No backups yet">
        <template #body="{ rows }">
          <tr v-for="b in (rows as BackupInfo[])" :key="b.id" class="hover:bg-gray-50">
            <td class="px-6 py-4 text-sm font-medium text-gray-900">{{ b.filename }}</td>
            <td class="px-6 py-4 text-sm text-gray-500">{{ formatSize(b.size) }}</td>
            <td class="px-6 py-4 text-sm text-gray-500">{{ formatDate(b.created_at) }}</td>
            <td class="px-6 py-4 text-sm text-right">
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
    <div v-if="restoreTarget" class="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl p-6 w-full max-w-md mx-4">
        <h3 class="text-lg font-semibold text-gray-900 mb-2">Restore from Backup</h3>
        <p class="text-sm text-gray-500 mb-4">
          Restore <span class="font-medium">{{ restoreTarget.filename }}</span>
        </p>

        <div class="space-y-3 mb-5">
          <label class="flex items-start gap-3 cursor-pointer p-3 rounded-lg border transition-colors"
            :class="restoreMode === 'merge' ? 'border-indigo-500 bg-indigo-50' : 'border-gray-200 hover:border-gray-300'">
            <input type="radio" v-model="restoreMode" value="merge" class="mt-0.5 text-indigo-600" />
            <div>
              <p class="text-sm font-medium text-gray-800">Merge</p>
              <p class="text-xs text-gray-500">Add contacts from backup that don't exist yet (safe, non-destructive)</p>
            </div>
          </label>

          <label class="flex items-start gap-3 cursor-pointer p-3 rounded-lg border transition-colors"
            :class="restoreMode === 'replace' ? 'border-red-500 bg-red-50' : 'border-gray-200 hover:border-gray-300'">
            <input type="radio" v-model="restoreMode" value="replace" class="mt-0.5 text-red-600" />
            <div>
              <p class="text-sm font-medium text-gray-800">Replace</p>
              <p class="text-xs text-gray-500 text-red-600">
                Delete ALL current contacts and restore from backup. This cannot be undone.
              </p>
            </div>
          </label>
        </div>

        <p v-if="restoreResult" class="text-sm text-green-700 bg-green-50 rounded p-2 mb-3">
          Done: {{ restoreResult.imported }} imported, {{ restoreResult.skipped }} skipped, {{ restoreResult.errors }} errors.
        </p>
        <p v-if="restoreError" class="text-sm text-red-600 mb-3">{{ restoreError }}</p>

        <div class="flex justify-end gap-3">
          <button
            @click="restoreTarget = null; restoreResult = null; restoreError = ''"
            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
          >
            Cancel
          </button>
          <AppButton
            :loading="restoring"
            :variant="restoreMode === 'replace' ? 'danger' : 'primary'"
            @click="handleRestore"
          >
            Restore
          </AppButton>
        </div>
      </div>
    </div>

    <!-- Delete confirmation modal -->
    <div v-if="deleteTarget" class="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div class="bg-white rounded-lg shadow-xl p-6 w-full max-w-sm mx-4">
        <h3 class="text-lg font-semibold text-gray-900 mb-2">Delete Backup</h3>
        <p class="text-sm text-gray-500 mb-4">
          Delete <span class="font-medium">{{ deleteTarget.filename }}</span>? This cannot be undone.
        </p>
        <div class="flex justify-end gap-3">
          <button
            @click="deleteTarget = null"
            class="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded-md hover:bg-gray-50"
          >
            Cancel
          </button>
          <AppButton variant="danger" :loading="deleting" @click="handleDelete">Delete</AppButton>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  createBackup, listBackups, downloadBackup, deleteBackup,
  restoreBackup, getBackupSettings, saveBackupSettings,
} from '@/api/backup'
import type { BackupInfo, BackupSettings, RestoreResult } from '@/types'
import AppButton from '@/components/ui/AppButton.vue'
import AppCard from '@/components/ui/AppCard.vue'
import AppTable from '@/components/ui/AppTable.vue'

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
    await createBackup()
    await load()
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
const restoring = ref(false)
const restoreResult = ref<RestoreResult | null>(null)
const restoreError = ref('')

function openRestore(b: BackupInfo) {
  restoreTarget.value = b
  restoreMode.value = 'merge'
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
    restoreError.value = (e as Error)?.message ?? 'Restore failed'
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

interface Preset { label: string; value: string }

const schedulePresets: Preset[] = [
  { label: 'Daily at 2 AM',           value: '0 2 * * *' },
  { label: 'Daily at midnight',        value: '0 0 * * *' },
  { label: 'Weekly (Sun 2 AM)',        value: '0 2 * * 0' },
  { label: 'Monthly (1st at 2 AM)',    value: '0 2 1 * *' },
  { label: 'Custom',                   value: 'custom' },
]

const activePreset = ref('0 2 * * *')

function applyPreset(preset: Preset) {
  activePreset.value = preset.value
  if (preset.value !== 'custom') {
    settings.value.schedule = preset.value
  }
}

function syncActivePreset(schedule: string) {
  const match = schedulePresets.find(p => p.value === schedule && p.value !== 'custom')
  activePreset.value = match ? match.value : 'custom'
}

async function loadSettings() {
  try {
    const { data } = await getBackupSettings()
    settings.value = data
    syncActivePreset(data.schedule)
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
    setTimeout(() => { settingsSaved.value = false }, 3000)
  } finally {
    savingSettings.value = false
  }
}

// ── Lifecycle ──────────────────────────────────────────────────────────────
onMounted(async () => {
  await Promise.all([load(), loadSettings()])
})

// ── Helpers ────────────────────────────────────────────────────────────────
function formatSize(bytes: number): string {
  if (bytes < 1024) return bytes + ' B'
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
  return (bytes / 1024 / 1024).toFixed(1) + ' MB'
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString('en-US', {
    year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}
</script>
